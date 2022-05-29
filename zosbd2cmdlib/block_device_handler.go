// SPDX-License-Identifier: LGPL-2.1
// Copyright (C) 2021-2022 stu mark

/* This module handles the (at the moment) single threaded shuttling of data between the block device
via it's handle_id to the Disk thing it is passed. */

package zosbd2cmdlib

import (
	"os"
	"syscall"
	"time"

	"github.com/nixomose/nixomosegotools/tools"
	"github.com/nixomose/zosbd2goclient/zosbd2cmdlib/zosbd2interfaces"
)

type Block_device_handler struct {
	m_log                    *tools.Nixomosetools_logger
	m_kmod                   *Kmod
	m_storage                zosbd2interfaces.Storage_mechanism // interfaces in go are pointers, never forget that.
	m_device_name            string
	m_block_device_handle_id uint32
	m_block_device_fd        *os.File
}

func New_block_device_handler(log *tools.Nixomosetools_logger, kmod *Kmod, device_name string,
	storage zosbd2interfaces.Storage_mechanism, handle_id uint32) Block_device_handler {
	var b Block_device_handler
	b.m_log = log
	b.m_kmod = kmod
	b.m_device_name = device_name
	b.m_storage = storage
	b.m_block_device_handle_id = handle_id
	b.m_block_device_fd = nil
	return b
}

func (this *Block_device_handler) Run_block_for_request(fd *os.File, op *zosbd2_operation, kernelbuffer []byte) tools.Ret {

	/* This is the center of the waiting universe, if this fails, we just keep trying until we can prove
	   it'll never succeed, like the device is gone.
	   It has to work, which is why it doesn't return anything.
	   Well, okay it will only return if we need to exit, which for now is never, until we have
	   a good list of things that are permanent failures. */
	for {
		var r tools.Ret = this.m_kmod.Block_for_request(fd, this.m_block_device_handle_id, op, kernelbuffer)
		/* Ahhh, what to do if this fails, wait and try again... */
		/* So it turns out there are cases when we do want to bail, there are some permanent failures, like:
		 * block device doesn't exist. For example, somebody sends the kernel module a request to end this block device.
		 * the kernel module will queue an exit request to userspace, but if userspace takes too long to come back, it will
		 * cancel that request and destroy the block device. the userspace thing never gets the exit request so it goes
		 * to respond to its last request, and fails because the block device is no longer there.
		 * Then it comes here and spins forever trying to contact the block device which will never return.
		 * In that case, we should exit. So we check for the actual error being ENOENT to fail. We should
		 * probably default to failing except in cases where we know something is retryable, but this will do for now. */
		if r != nil {
			if (r.Get_errcode() == int(syscall.ENOENT)) || (r.Get_errcode() == -int(syscall.ENOENT)) {
				this.m_log.Error("got ENOENT on block device trying to block for request, failing out. error: ", r.Get_errmsg())
				return r
			}
			this.m_log.Error("Error from block_for_request, sleeping 1 second, error: ", r.Get_errmsg())
			time.Sleep(1 * time.Second)
		} else {
			break
		}
	}
	return nil
}

func (this *Block_device_handler) Run() tools.Ret {
	/* So this is the main loop of the block device handler. We initially block waiting for a request and then handle them
	   as they come in, and so on, until we get an 'exit' request from the kernel. */

	/* first thing we do is open our shiney new block device */
	var full_device_name string = TXT_DEVICE_PATH + this.m_device_name
	// var fd *os.File
	var ret tools.Ret
	ret, this.m_block_device_fd = this.m_kmod.Open_bd(full_device_name)
	if ret != nil {
		return tools.ErrorWithCode(this.m_log, ret.Get_errcode(), "Error opening block device: ", full_device_name, ", handle id: ", this.m_block_device_handle_id, ", error: ", ret.Get_errmsg())
	}

	// the size of the buffer we're going to pass to the kernel. the structure plus the userspacedatabuffer at the end.
	/* so in C this is easy because it's one buffer and we control it. in go, it is possible for the runtime
	   to shuffle it around at will. it is okay for the buffer to be moved while it's in go-land, but when it's
		 in the kernel it's gotta stay put. it's not required that we pass in the same buffer to the kernel
		 each time, but it makes things easier, because we get the buffer back filled in with the request from the
		 kernel, we could make a new buffer for the next call, but the way the program is laid out at the moment
		 it's just easier to keep using the same buffer all the time, and if go moves it around, no big deal. */
	/* so size of zosbd2_operation comes up with 4160 + metadata, and it should be 4152 + metadata. and I think
	   it's coming from the size of the go struct and not what it will actually serialize to, so let's try serializing
		 and take the size of that. */

	var workarea zosbd2_operation
	// var sizeof = unsafe.Sizeof(sizer)
	// var alloc_size int = int(sizeof) + MAX_BIO_SEGMENTS_BUFFER_SIZE_PER_REQUEST // this is currently hardcoded as the block size 4k * the max number of blocks we can ask for it one list of segment buffers

	var alloc_size int
	ret, alloc_size = this.m_kmod.Get_op_size()
	if ret != nil {
		return tools.Error(this.m_log, "Error calculating operation buffer size, error: ", ret.Get_errmsg())
	}
	/* okay so that got me 4148 which is 4 short of 4152 which means we're missing padding at the end somewhere.
	   turns out we were missing the padding at the end of the zosbd2_packet structure. it's 8 byte aligned
		 and 12 isn't divisible by 8 so we pad up to 16 and then we get our 4152. */

	// https://stackoverflow.com/questions/54388088/how-to-ioctl-properly-from-golang
	/* in go in order to allocate memory and have it be safe from being moved by the go runtime you have to call a syscall
	   with one of the parameters being the unsafe call...

	   // good/valid:
	   syscall.Syscall(SYS_READ, uintptr(fd), uintptr(unsafe.Pointer(p)), uintptr(n))

	   The compiler handles a Pointer converted to a uintptr in the argument list of a call to a function implemented in assembly by
	   arranging that the referenced allocated object, if any, is retained and not moved until the call completes, even though from
	   the types alone it would appear that the object is no longer needed during the call.

	   For the compiler to recognize this pattern, the conversion must appear in the argument list:

	   // INVALID: uintptr cannot be stored in variable  before implicit conversion back to Pointer during system call.
	   u := uintptr(unsafe.Pointer(p))
	   syscall.Syscall(SYS_READ, uintptr(fd), u, uintptr(n))

	   that guarantees that the memory wont move for the life of the syscall
	   if you need memory to exist over time over multiple function calls you're better off having a cgo function
	   allocate the memory with malloc and manually free it later because if it's a go managed variable, it will get
	   garbage collected or moved. */

	/* so I believe all that ends up meaning that we can make a buffer here, and the go runtime can move it all
	around but as long as it stays still for the duration of the ioctl (which can be very long remember)
	we should be good. This is THE buffer used to shuttle requests to and from the kernel forever. */
	//	var buffer *bytes.Buffer = bytes.NewBuffer(make([]byte, 0, alloc_size))
	var kernelbuffer []byte = make([]byte, alloc_size)
	if kernelbuffer == nil { // I seem to recall somebody saying this can't fail in go.
		return tools.ErrorWithCode(this.m_log, -int(syscall.ENOMEM), "Unable to allocate memory for kernel zosbd2_operation: ", syscall.ENOMEM)
	}

	/* here is the problem, buffer is the serialized size of zosbd2_operation
		 op is a pointer to that buffer with the type of zosbd2_operation and while in C they would
	   line up, in go they do not, you can't lay them on top, we must treat them separately,
	   modify variables in op and serialize to a buffer and copy to kerne; buffer and submit kernel buffer to ioctl */

	var op *zosbd2_operation = &workarea //(*zosbd2_operation)(unsafe.Pointer(&buffer[0]))

	// get our first request from the kernel
	ret = this.Run_block_for_request(this.m_block_device_fd, op, kernelbuffer)
	if ret != nil { // this will never happen, but if it does, it is a permanent failure
		return tools.ErrorWithCode(this.m_log, int(syscall.EINVAL), "permanent failure from block_for_request, bailing out. error: ", ret.Get_errmsg())
	}

	// we got our first request, loop forever handling all the requests.
	for {
		// switch on the type of operation, all functions block until a new request comes in from the kernel.
		/* The kernel module never returns us an error if something on its side failed, it returns the block layer
		   the error. To us, it only calls with requests, so if we get a failure for a request, then it's just something we
		   have to retry forever. If the block device disappears somehow, then that will be a permanent failure and we should
		   check for those kinds of things and exit. */
		ret = nil
		switch op.Header.Operation {
		case DEVICE_OPERATION_KERNEL_BLOCK_FOR_REQUEST:
			{
				ret = this.Run_block_for_request(this.m_block_device_fd, op, kernelbuffer)
				break
			}
		case DEVICE_OPERATION_KERNEL_READ_REQUEST:
			{
				ret = this.m_kmod.Respond_to_read_request(this.m_block_device_fd, this.m_block_device_handle_id, op, kernelbuffer, this.m_storage)
				break
			}
		case DEVICE_OPERATION_KERNEL_WRITE_REQUEST:
			{
				ret = this.m_kmod.Respond_to_write_request(this.m_block_device_fd, this.m_block_device_handle_id, op, kernelbuffer, this.m_storage)
				break
			}
		case DEVICE_OPERATION_KERNEL_DISCARD_REQUEST:
			{
				ret = this.m_kmod.respond_to_discard_request(this.m_block_device_fd, this.m_block_device_handle_id, op, kernelbuffer, this.m_storage)
				break
			}
		case DEVICE_OPERATION_KERNEL_USERSPACE_EXIT:
			{
				this.m_log.Info("got request from kernel to exit. exiting cleanly.")
				return nil
			}
		default:
			{
				this.m_log.Error("Unsupported request from kernel module: ", op.Header.Operation)
				ret = tools.ErrorWithCode(this.m_log, -int(syscall.EINVAL), "invalid request, blocking for another request.")
				break
			}
		}

		/* check to see if we got an error on the block_for_request part, if so, as above, sleep and try again.
		   later version will deal with different kinds of errors more appropriately, exiting or retrying.
		   for example if the block device disappears, it's never going to come back, and asking it for things
		   will be forever pointless. */

		if ret != nil {
			this.m_log.Error("error from block_for_request call, sleeping and blocking for another request: ", ret.Get_errmsg())
			ret = this.Run_block_for_request(this.m_block_device_fd, op, kernelbuffer)
			if ret != nil {
				// this will never happen, but if it does, it is a permanent failure
				return tools.ErrorWithCode(this.m_log, -int(syscall.EINVAL), "permanent failure from block_for_request, bailing out. error: ", ret.Get_errmsg())
			}
		}
	} // loop forever
}
