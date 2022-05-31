// SPDX-License-Identifier: LGPL-2.1
// Copyright (C) 2021-2022 stu mark

package zosbd2cmdlib

import (
	"bytes"
	"encoding/binary"
	"strings"

	//	"encoding/binary"
	"os"
	"syscall"
	"unsafe"

	"github.com/nixomose/nixomosegotools/tools"
	"github.com/nixomose/zosbd2goclient/zosbd2cmdlib/zosbd2interfaces"
	//	"github.com/augustoroman/hexdump"
)

const TXT_DEVICE_PATH string = "/dev/"
const CONTROL_DEVICE_NAME string = "/dev/zosbd2ctl"

const device_filemode os.FileMode = 0666

type Kmod struct {
	m_log       *tools.Nixomosetools_logger
	m_sizeof_op int
}

type Device_status struct {
	/* this struct represents the go version of the status for one device */
	Size                     uint64 `json:"size_in_bytes"`              // size of the block device in bytes
	Number_of_blocks         uint64 `json:"number_of_blocks"`           // this times kernel_block_size is the total device size in bytes.
	Kernel_block_size        uint32 `json:"kernel_block_size_in_bytes"` // saving for reference, we don't really need to keep this, this is 4k
	Max_segments_per_request uint32 `json:"max_segments_per_request"`   // how many segments the kmod should return to userspace per request.
	Timeout_milliseconds     uint32 `json:"timeout_in_milliseconds"`    // number of milliseconds before the block device gives up waiting for userspace to respond. kernel uses ms, converted from seconds on the way in
	Handle_id                uint32 `json:"handle_id"`                  // so they can check what we gave them is what they asked for
	Device_name              string `json:"device_name"`                // the name of the device in /dev, +1 for nt.
}

func New_kmod(log *tools.Nixomosetools_logger) Kmod {
	var k Kmod
	k.m_log = log
	k.m_sizeof_op = 0
	return k
}

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

func (this *Kmod) Get_op_size() (tools.Ret, int) {
	var workarea zosbd2_operation
	structbuf := &bytes.Buffer{}
	err := binary.Write(structbuf, binary.LittleEndian, workarea)
	if err != nil {
		return tools.ErrorWithCode(this.m_log, -int(syscall.EINVAL), "unable to serialize zosbd2_operation"), 0
	}
	var sizeof = structbuf.Len()

	/* sizeof also happens to be the position of the actual databuffer in kernelbuffer (4152) that read and write need to know,
	   so save it now that we know it and pass it in to read and write so they don't have to serialize op again just
		 to find out how big it is. */
	this.m_sizeof_op = sizeof

	var alloc_size int = int(sizeof) + MAX_BIO_SEGMENTS_BUFFER_SIZE_PER_REQUEST // this is currently hardcoded as the block size 4k * the max number of blocks we can ask for it one list of segment buffers

	return nil, alloc_size
}

/*****************************************************************************************************/
/*                helpers to deal with getting op and data into and out of kernelbuffer              */
/*****************************************************************************************************/

// func (k *Kmod) datatokernelbuffer(data []byte, kernelbuffer []byte) tools.Ret {
// 	// copy this data to the userspacedatabufferpos of kernelbuffer
// 	var copied int = copy(kernelbuffer[k.m_sizeof_op:], data)
// 	if copied != len(data) {
// 		return tools.ErrorWithCode(k.m_log, -int(syscall.EINVAL), "unable to copy data to kernel buffer, ",
// 			"copied: ", copied, " expected:", len(data))
// 	}
// 	return nil
// }

// func (k *Kmod) kernelbuffertodata(data []byte, kernelbuffer []byte) tools.Ret {
// 	// copy  the userspacedatabufferpos portion of kernelbuffer to data
// 	var copied int = copy(data, kernelbuffer[k.m_sizeof_op:])
// 	if copied != len(data) {
// 		return tools.ErrorWithCode(k.m_log, -int(syscall.EINVAL), "unable to copy kernelbuffer data to caller data buffer, ",
// 			"copied: ", copied, " expected:", len(data))
// 	}
// 	return nil
// }

func (this *Kmod) optokernelbuffer(op *zosbd2_operation, kernelbuffer []byte) tools.Ret {
	/* take the op struct and serialize it to the first  4152 bytes of kernelbuffer, does not steal op */
	structbuf := &bytes.Buffer{}
	err := binary.Write(structbuf, binary.LittleEndian, op)
	if err != nil {
		return tools.ErrorWithCode(this.m_log, -int(syscall.EINVAL), "unable to serialize block for request struct")
	}

	var b []byte = structbuf.Bytes()
	var copied int = copy(kernelbuffer, b)
	if copied != this.m_sizeof_op {
		return tools.ErrorWithCode(this.m_log, -int(syscall.EINVAL), "unable to copy request struct to kernel buffer, ",
			"copied: ", copied, " expected:", this.m_sizeof_op)
	}
	return nil
}

func (this *Kmod) kernelbuffertoop(op *zosbd2_operation, kernelbuffer []byte) tools.Ret {
	/* take the first 4152 bytes of kernelbuffer and deserialize it to op */
	/* we don't want bytes.newbuffer to steal our kernelbuffer, so we have to copy the 4152 out first */
	var opspace []byte = make([]byte, this.m_sizeof_op) // this only has to be as big as the serialized version
	var copied int = copy(opspace, kernelbuffer[:this.m_sizeof_op])

	if copied != this.m_sizeof_op {
		return tools.ErrorWithCode(this.m_log, -int(syscall.EINVAL), "unable to copy kernel buffer op struct to temp buffer, ",
			"copied: ", copied, " expected:", this.m_sizeof_op)
	}

	var bback *bytes.Buffer = bytes.NewBuffer(opspace) // so it can be stolen from opspace safely without messing up kernelbuffer
	var err error = binary.Read(bback, binary.LittleEndian, op)
	if err != nil {
		return tools.ErrorWithCode(this.m_log, -int(syscall.EINVAL), "unable to deserialize zosbd2 operation struct to op:", err.Error())
	}

	return nil
}

/*****************************************************************************************************/
/*                                            ioctl helpers                                          */
/*****************************************************************************************************/

func (this *Kmod) Open_bd(device_name string) (tools.Ret, *os.File) {
	var fdout *os.File
	var err error
	fdout, err = os.OpenFile(device_name, os.O_RDWR, device_filemode)
	if err != nil {
		return tools.Error(this.m_log, "unable to open block control device ", device_name, ", err: ", err,
			". Did you load the zosbd2 kernel module? clone and install from here: https://github.com/nixomose/zosbd2"), nil
	}
	return nil, fdout
}

func (this *Kmod) Close_bd(device_handle *os.File) tools.Ret {
	var err error = device_handle.Close()
	if err != nil {
		return tools.Error(this.m_log, "unable to close block control device ", device_handle, ", err: ", err)
	}
	return nil
}

func (this *Kmod) Safe_ioctl(fd *os.File, cmd uint64, data []byte) tools.Ret {

	// a, b, ep := syscall.Syscall(syscall.SYS_IOCTL, fd, op, arg)
	var errno syscall.Errno
	var retval uintptr
	// r1 is actual return value, but we don't use it. we use errno
	/* this worked but is not guaranteed, we must convert the go object
	to a unsafe pointer and cast to uintptr inline in one call so the garbage collector
	can't move anything out from underneath us...
		retval, _, errno = syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd.Fd()), uintptr(cmd), uintptr(data)) */

	// bytes (data) is a slice, get the address of the first member of the slice, not the slice header.
	if len(data) == 0 {
		data = make([]byte, 1) // so we have something to get the address of for the unsafe pointer.
	}

	var backup = append([]byte{}, data...) // in case we have to retry

	for {
		retval, _, errno = syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd.Fd()), uintptr(cmd), uintptr(unsafe.Pointer(&(data[0]))))
		/* when we make an ioctl call and get EINTR, the kmod has modified the calling buffer with the response so we can't just resubmit
		the syscall again without resetting the request. But I think we have enough information to do that with. */

		if errno != 0 {
			if errno == syscall.EINTR {
				this.m_log.Debug("got EINTR from syscall, retrying", fd, " cmd ", cmd, " err: ", errno)
				copy(data, backup) // put back the original request that the kmod might have overwritten
				continue
			}
		}

		if int(retval) != 0 {
			/* This will cause the block for request forever-loop to exit, which seems a bit draconian
			if we can't cover all possible failures. So for now we'll leave it, but maybe fix it
			if we crash for no good reason too much. */
			return tools.ErrorWithCode(this.m_log, int(errno), "unable to make ioctl call for fd: ", fd, " cmd ", cmd, " err: ", errno)
		}
		break
	}
	return nil
}

/*****************************************************************************************************/
/*                                        control device ioctls                                      */
/*****************************************************************************************************/

func (this *Kmod) Control_ping(fd *os.File) tools.Ret {
	var co control_ping

	this.m_log.Info("Pinging ", CONTROL_DEVICE_NAME)

	structbuf := &bytes.Buffer{}
	err := binary.Write(structbuf, binary.LittleEndian, co)
	if err != nil {
		return tools.ErrorWithCode(this.m_log, -int(syscall.EINVAL), "unable to serialize ping struct")
	}
	var b []byte = structbuf.Bytes()

	var ret = this.Safe_ioctl(fd, IOCTL_CONTROL_PING, b) // nothing to read back for ping
	if ret != nil {
		return tools.ErrorWithCode(this.m_log, ret.Get_errcode(), "ioctl call for control ping failed, err: ", ret.Get_errcode())
	}
	this.m_log.Info("Ping ", CONTROL_DEVICE_NAME, " successful.")

	return nil
}

func (this *Kmod) Get_status_device_name(cs *control_status_device_request_response) (tools.Ret, string) {
	var s string
	i := bytes.IndexByte(cs.Device_name[:], 0)
	if i == -1 {
		return tools.Error(this.m_log, "Invalid response from kernel, no valid device name."), ""
	}
	s = string(cs.Device_name[0:i])
	return nil, s
}

func (this *Kmod) Set_create_params_device_name(co *control_block_device_create_params, device_name string) tools.Ret {

	if len(device_name) > MAX_DEVICE_NAME_LENGTH {
		return tools.ErrorWithCode(this.m_log, int(syscall.ENAMETOOLONG), "block device name: ", device_name, " is longer than ", MAX_DEVICE_NAME_LENGTH, " characters.")
	}
	var min = tools.Minint(len(device_name), MAX_DEVICE_NAME_LENGTH)
	copy(co.Device_name[0:min], device_name)
	co.Device_name[len(device_name)] = 0 // NT
	return nil
}

func (this *Kmod) set_destroy_by_name_params_device_name(co *control_block_device_destroy_by_name_params, device_name string) tools.Ret {
	if len(device_name) > MAX_DEVICE_NAME_LENGTH {
		return tools.ErrorWithCode(this.m_log, int(syscall.ENAMETOOLONG), "block device name: ", device_name, " is longer than ", MAX_DEVICE_NAME_LENGTH, " characters.")
	}

	var min = tools.Minint(len(device_name), MAX_DEVICE_NAME_LENGTH)
	copy(co.Device_name[0:min], device_name)
	co.Device_name[len(device_name)] = 0 // NT
	return nil
}

func (this *Kmod) Create_block_device(fd *os.File, device_name string, block_size uint32, number_of_kernel_blocks uint64,
	device_timeout_seconds uint32) (ret tools.Ret, handle_id_out uint32) {
	this.m_log.Info("creating block device: ", device_name, ", kernel block size: ", block_size, ", number of blocks: ", number_of_kernel_blocks,
		", total device size bytes: ", uint64(block_size)*uint64(number_of_kernel_blocks), ", device timeout seconds: ", device_timeout_seconds)

	var op_create control_block_device_create_params

	this.Set_create_params_device_name(&op_create, device_name)
	op_create.Kernel_block_size = block_size
	op_create.Number_of_blocks = number_of_kernel_blocks
	op_create.Device_timeout_seconds = device_timeout_seconds
	op_create.Max_segments_per_request = 0     // zero is max
	op_create.Bio_segment_batch_queue_size = 0 // 1/22/2021 don't use this yet...

	// var sizeof_op = unsafe.Sizeof(op_create)
	// k.m_log.Debug("sizeof op: ", sizeof_op)
	// var sizeof_device_name = unsafe.Sizeof(op_create.device_name)
	// k.m_log.Debug("sizeof device_name: ", sizeof_device_name)

	structbuf := &bytes.Buffer{}
	err := binary.Write(structbuf, binary.LittleEndian, op_create)
	if err != nil {
		return tools.ErrorWithCode(this.m_log, -int(syscall.EINVAL), "unable to serialize create struct"), 0
	}
	// k.m_log.Debug("sizeof struct:", unsafe.Sizeof(op_create), " sizeof structbuf: ", unsafe.Sizeof((*structbuf).Bytes()),
	// 	" len of buf: ", structbuf.Len())

	var b []byte = structbuf.Bytes()
	//	k.m_log.Debug(hexdump.Dump(structbuf.Bytes()))

	var r = this.Safe_ioctl(fd, IOCTL_CONTROL_CREATE_DEVICE, b)

	bback := bytes.NewBuffer(b) // takes ownership of b

	/* not super required, but even if it fails, it is possible the kernel sent back something in the op_create.error
		   field and we can deserialize and get it out, before we even check if the ioctl worked, because we have to do this
	 		 whether it works or not */

	err = binary.Read(bback, binary.LittleEndian, &op_create)
	if err != nil {
		return tools.ErrorWithCode(this.m_log, -int(syscall.EINVAL), "unable to deserialize zosbd2 operation struct response after syscall:", err.Error()), 0
	}
	if r != nil {
		return tools.ErrorWithCode(this.m_log, r.Get_errcode(), "ioctl call to create block device failed, errorcode ", r.Get_errcode()), 0
	}

	/* If we got here, the device was made and we got a handle id back */
	var handle_id uint32 = op_create.Handle_id
	this.m_log.Info("create block device for ", device_name, " successful, handle_id: ", handle_id)

	return nil, handle_id
}

func (this *Kmod) Destroy_block_device_by_id(fd *os.File, handle_id uint32) tools.Ret {
	this.m_log.Info("destroying block device with handle_id: ", handle_id)

	var op_destroy control_block_device_destroy_by_id_params
	op_destroy.Handle_id = handle_id
	op_destroy.Force = 1

	structbuf := &bytes.Buffer{}
	err := binary.Write(structbuf, binary.LittleEndian, op_destroy)
	if err != nil {
		return tools.ErrorWithCode(this.m_log, -int(syscall.EINVAL), "unable to serialize destroy struct")
	}
	var b []byte = structbuf.Bytes()
	var r = this.Safe_ioctl(fd, IOCTL_CONTROL_DESTROY_DEVICE_BY_ID, b)

	bback := bytes.NewBuffer(b)
	err = binary.Read(bback, binary.LittleEndian, &op_destroy)
	if err != nil {
		return tools.ErrorWithCode(this.m_log, -int(syscall.EINVAL), "unable to deserialize zosbd2 operation struct response after syscall:", err.Error())
	}

	if r != nil {
		return tools.ErrorWithCode(this.m_log, r.Get_errcode(), "ioctl call to destroy block device by id failed, errorcode ",
			r.Get_errcode(), ", destroy error: ", op_destroy.Error)
	}
	/* If we got here, we're good. */
	this.m_log.Info("destroy block device for handle_id ", handle_id, " successful")

	return nil
}

func (this *Kmod) Destroy_block_device_by_name(fd *os.File, device_name string) tools.Ret {
	this.m_log.Info("destroying block device with device name: ", device_name)

	var op_destroy control_block_device_destroy_by_name_params
	this.set_destroy_by_name_params_device_name(&op_destroy, device_name)
	op_destroy.Force = 1

	structbuf := &bytes.Buffer{}
	err := binary.Write(structbuf, binary.LittleEndian, op_destroy)
	if err != nil {
		return tools.ErrorWithCode(this.m_log, -int(syscall.EINVAL), "unable to serialize destroy struct")
	}
	var b []byte = structbuf.Bytes()

	var r = this.Safe_ioctl(fd, IOCTL_CONTROL_DESTROY_DEVICE_BY_NAME, b)

	bback := bytes.NewBuffer(b)
	err = binary.Read(bback, binary.LittleEndian, &op_destroy)
	if err != nil {
		return tools.ErrorWithCode(this.m_log, -int(syscall.EINVAL), "unable to deserialize zosbd2 operation struct response after syscall:", err.Error())
	}

	if r != nil {
		return tools.ErrorWithCode(this.m_log, r.Get_errcode(), "ioctl call to destroy block device by name failed, errorcode ", r.Get_errcode(), ", destroy error: ", op_destroy.Error)
	}
	/* If we got here, we're good. */
	this.m_log.Info("destroy block device for device name ", device_name, " successful")

	return nil
}

func (this *Kmod) Destroy_all_block_devices(fd *os.File) tools.Ret {
	this.m_log.Info("destroying all block devices.")

	var op_destroy control_block_device_destroy_all_params

	op_destroy.Force = 1 // until we have a command line param, always force

	structbuf := &bytes.Buffer{}
	err := binary.Write(structbuf, binary.LittleEndian, op_destroy)
	if err != nil {
		return tools.ErrorWithCode(this.m_log, -int(syscall.EINVAL), "unable to serialize destroy struct")
	}
	var b []byte = structbuf.Bytes()

	var r = this.Safe_ioctl(fd, IOCTL_CONTROL_DESTROY_ALL_DEVICES, b)
	bback := bytes.NewBuffer(b)
	err = binary.Read(bback, binary.LittleEndian, &op_destroy)
	if err != nil {
		return tools.ErrorWithCode(this.m_log, -int(syscall.EINVAL), "unable to deserialize zosbd2 operation struct response after syscall:", err.Error())
	}

	if r != nil {
		return tools.ErrorWithCode(this.m_log, r.Get_errcode(), "ioctl call to destroy all block devices failed, errorcode ", r.Get_errcode())
	}
	/* If we got here, we're good. */
	this.m_log.Info("destroy all block devices successful")

	return nil
}

func (this *Kmod) get_device_status(fd *os.File, handle_id uint32) (tools.Ret, *Device_status) {
	this.m_log.Info("getting status for handle_id ", handle_id)

	var op_status_device control_status_device_request_response
	op_status_device.Handle_id_request = handle_id

	structbuf := &bytes.Buffer{}
	err := binary.Write(structbuf, binary.LittleEndian, op_status_device)
	if err != nil {
		return tools.ErrorWithCode(this.m_log, -int(syscall.EINVAL), "unable to serialize status device request struct"), nil
	}

	var b []byte = structbuf.Bytes()

	var r = this.Safe_ioctl(fd, IOCTL_CONTROL_STATUS_DEVICE_STATUS, b)

	bback := bytes.NewBuffer(b) // takes ownership of b

	err = binary.Read(bback, binary.LittleEndian, &op_status_device)
	if err != nil {
		return tools.ErrorWithCode(this.m_log, -int(syscall.EINVAL), "unable to deserialize status device status struct response after syscall:", err.Error()), nil
	}
	if r != nil {
		return tools.ErrorWithCode(this.m_log, r.Get_errcode(), "ioctl call to get status device failed, errorcode ", r.Get_errcode()), nil
	}

	// now copy all the fields out into something go-ish we can return
	var device_status Device_status
	device_status.Size = op_status_device.Size
	device_status.Number_of_blocks = op_status_device.Number_of_blocks
	device_status.Kernel_block_size = op_status_device.Kernel_block_size
	device_status.Max_segments_per_request = op_status_device.Max_segments_per_request
	device_status.Timeout_milliseconds = op_status_device.Timeout_milliseconds
	device_status.Handle_id = op_status_device.Handle_id
	var ret tools.Ret
	ret, device_status.Device_name = this.Get_status_device_name(&op_status_device)

	return ret, &device_status

}

func (this *Kmod) Get_devices_status_map(fd *os.File) (tools.Ret, map[string]Device_status) {
	/* call kmod to get the count of devices and the list of their handle_ids.
	then call the kmod for each one filling up the device status map. the map is keyed off
	device_name. all keys are returned in lower case */

	this.m_log.Info("getting status for all block devices.")

	/* first get the list */

	var op_status_list control_status_device_list_request_response
	// there are no params for this.

	// k.Set_create_params_device_name(&op_status, device_name)

	structbuf := &bytes.Buffer{}
	err := binary.Write(structbuf, binary.LittleEndian, op_status_list)
	if err != nil {
		return tools.ErrorWithCode(this.m_log, -int(syscall.EINVAL), "unable to serialize status device list request struct"), nil
	}

	var b []byte = structbuf.Bytes()

	var r = this.Safe_ioctl(fd, IOCTL_CONTROL_STATUS_DEVICE_LIST, b)

	bback := bytes.NewBuffer(b) // takes ownership of b

	err = binary.Read(bback, binary.LittleEndian, &op_status_list)
	if err != nil {
		return tools.ErrorWithCode(this.m_log, -int(syscall.EINVAL), "unable to deserialize status device list struct response after syscall:", err.Error()), nil
	}
	if r != nil {
		return tools.ErrorWithCode(this.m_log, r.Get_errcode(), "ioctl call to get status list failed, errorcode ", r.Get_errcode()), nil
	}

	/* If we got here, we have a list of device ids. */
	var count int = int(op_status_list.Num_devices)

	var device_map map[string]Device_status = make(map[string]Device_status)
	for lp := 0; lp < count; lp++ {
		var handle_id = op_status_list.Handleid[lp]
		// make the request for this handle_id
		var device_status *Device_status
		var ret tools.Ret
		ret, device_status = this.get_device_status(fd, handle_id)
		if ret != nil {
			return ret, nil
		}
		// otherwise add it to the map
		var key = strings.ToLower(device_status.Device_name)
		device_map[key] = *device_status
	}

	return nil, device_map
}

/*****************************************************************************************************/
/*                      the actual block device read/write/trim/etc io handlers                      */
/*****************************************************************************************************/

func (this *Kmod) Validate_read_request(handle_id uint32, read_request read_request_param_t) tools.Ret {
	// validate this request is for this handle_id

	if read_request.Number_of_segments == 0 {
		return tools.ErrorWithCode(this.m_log, int(syscall.EINVAL), "invalid read request, number of segments is zero.")
	}
	if read_request.Number_of_segments > MAX_BIO_SEGMENTS_PER_REQUEST {
		return tools.ErrorWithCode(this.m_log, int(syscall.EINVAL), "invalid read request, number of segments is greater than ", MAX_BIO_SEGMENTS_PER_REQUEST)
	}
	// need more here xxxz
	return nil
}

func (this *Kmod) Validate_write_request(handle_id uint32, write_request write_request_param_t) tools.Ret {
	return nil
}

func (this *Kmod) Validate_discard_request(handle_id uint32, discard_request discard_request_param_t) tools.Ret {
	return nil
}

func (this *Kmod) Block_for_request(fd *os.File, handle_id uint32, op *zosbd2_operation, kernelbuffer []byte) tools.Ret {
	// can't log much here alas because this happens all the time.
	op.Handle_id = handle_id
	op.Header.Operation = DEVICE_OPERATION_NO_RESPONSE_BLOCK_FOR_REQUEST
	op.Header.Size = 0

	var ret = this.optokernelbuffer(op, kernelbuffer)
	if ret != nil {
		return tools.ErrorWithCode(this.m_log, ret.Get_errcode(), "unable to convert zosbd2 operation to kernel buffer: ", ret.Get_errmsg())
	}

	var main_ret = this.Safe_ioctl(fd, IOCTL_DEVICE_OPERATION, kernelbuffer)

	/* so upon success OR failure we need to deserialize b back into op in case there's
	   an error code returned from the kernel. in the case of success we need to be able
	   to return whatever data the kernel responded with (like the handle_id of a newly
	   created device) to the caller in the op buffer they expect it back in. */

	/* sooooooooooooooo much copying. I think we're going to have to do this the cgo way eventually
	   anyway just because we serialize and deserialize and copy so many things soooooooooo many times... */
	ret = this.kernelbuffertoop(op, kernelbuffer)
	if ret != nil {
		return tools.ErrorWithCode(this.m_log, ret.Get_errcode(), "ioctl device operation call failed for block_for_request, error: ", ret.Get_errcode())
	}

	return main_ret
}

func (this *Kmod) validate_adjacency(requestop *zosbd2_operation, number_of_segments uint32) (retresp tools.Ret,
	entire_start_out uint64, entire_length_out uint32) {
	// 1/2/2021 as with read, make sure the write request is one big contiguous block.
	var verify_start_first bool = true
	var verify_start uint64 = 0
	var entire_start uint64 = 0
	var entire_length uint32 = 0

	var lp uint32
	for lp = 0; lp < number_of_segments; lp++ {
		var entry zosbd2_bio_segment_metadata = requestop.Metadata[lp]
		var start uint64 = entry.Start
		var length uint32 = uint32(entry.Length)

		if verify_start_first {
			verify_start_first = false
			verify_start = start
			entire_start = start // keep the location of the first segment as the actual start for the whole thing.
		} else {
			if verify_start != start {
				return tools.ErrorWithCode(this.m_log, -int(syscall.EINVAL), "metadata segments are not contiguous, expected start ", verify_start, " got start ", start), 0, 0
			}
		}
		verify_start += uint64(length)
		entire_length += length
	}
	return nil, entire_start, entire_length
}

func (this *Kmod) Respond_to_read_request(fd *os.File, handle_id uint32, requestop *zosbd2_operation, kernelbuffer []byte,
	storage zosbd2interfaces.Storage_mechanism) tools.Ret {
	/* Read what we need to respond with from the storage mechniams, generate response and call ioctl back with it */
	// 7/26/2020 uses new multi read interface
	/* 1/2/2021 so I think I introduced a bug when I did multiread/write because the file get and put
	 * is flawless with zos but the block device yields corruption with no errors after 7+ hours of
	 * constant use. Doesn't matter, turns out multiwrite wasn't necessary. We effectively removed it in zosserver
	 * by having multiwrite ensure that the sections were all adjacent and then just submitted the large
	 * read/write directly to the existing zos read write which was able to deal with block boundaries.
	 * Since here we break up the request into little pieces and then zosserver just mushes them back
	 * into one, I'm going to remove all the multread/write stuff and just ensure the request
	 * is adjacent here. Since it always is, it should just work. If we ever get a single bio request
	 * that has multiple non-adjacent sections, we'll just have to make multiple requests here.
	 * But that probably won't happen any time soon. */

	var ret = this.kernelbuffertoop(requestop, kernelbuffer)
	if ret != nil {
		return tools.ErrorWithCode(this.m_log, ret.Get_errcode(), "unable to get read request from kernelbuffer: ", ret.Get_errmsg())
	}

	var operation_id uint32 = requestop.Operation_id

	/* we don't want buffer to steal packet out of our struct, so we copy the contents, this may be unneccessary.
	it's a go managed buffer member of a go struct, no fancy business with unsafe pointers going on
	so maybe the reason they say not to use it after newbuffer is that it might change on you, but we're
	okay with that. */
	var packetspace []byte = make([]byte, zosbd2_packet_size_bytes)
	copy(packetspace, requestop.Packet[:])
	var buf *bytes.Buffer = bytes.NewBuffer(packetspace)

	var read_request read_request_param_t
	var err = binary.Read(buf, binary.LittleEndian, &read_request)
	if err != nil {
		return tools.ErrorWithCode(this.m_log, int(syscall.EINVAL), "invalid read request, unable to parse read request structure")
	}

	ret = this.Validate_read_request(handle_id, read_request)
	if ret != nil {
		return ret
	}

	var number_of_segments uint32 = read_request.Number_of_segments
	//        u64 total_length_of_all_bio_segments = requestop.packet.read_request.total_length_of_all_bio_segments; // in bytes, this is how much space we have to write in, to verify below... xxxz

	// we have to return a response object with the data in the databuffer
	/* requestop is the dynamically allocated memory that is the zosbd2_operation structure size + the transfer buffer at the end so we must take care to use the same
	   buffer, because we can not easily make another one. especially if these someday start to get randomly sized per request. */
	var responseop *zosbd2_operation = requestop

	responseop.Handle_id = handle_id
	responseop.Operation_id = operation_id // pass this back in as is.
	responseop.Error = 0                   // default good unless overridden by error later
	responseop.Header.Operation = DEVICE_OPERATION_READ_RESPONSE
	responseop.Header.Size = 0 // the amount of data in userspacedatabuffer that we return, we tally this up as we go along through the bio segments.

	/* 1/2/2021 there's an amusing history of comments in write_new_multi_2.
	 * the short version is we're going to make the read/write request as one large chunk and zos server will
	 * deal with breaking it up over block boundaries if it needs to just like it always has for get and put.
	 * right now we break it into pieces send the pieces over and it rebatches them back into one request.
	 * that's probably where the corruption bug is, so we're going to remove all references to multi anything
	 * and just do it the simplest way I wish I had done the first time around. (I realize the reason we didn't
	 * do it that way was because everything was hardcoded to 4k blocks as the blocksize on zos, so it wasn't obvious
	 * there was a better way to do it with bigger blocks.)
	 * We just need to ensure that the requests are all one big contiguous block.
	 * For now error if not, in the future if we ever actually see one in the wild, we'll have to make this
	 * multiple requests. */

	var entire_start uint64 = 0
	var entire_length uint32 = 0
	var adjret tools.Ret

	adjret, entire_start, entire_length = this.validate_adjacency(requestop, number_of_segments)
	if adjret != nil {
		return tools.ErrorWithCode(this.m_log, -int(syscall.EINVAL), "respond_to_write_request: ", adjret.Get_errmsg())
	}

	// xxxz check and make sure we're not going past total_length_of_all_bio_segments, that's the max size of the available buffer to fill.
	var total_buffer_size_tally uint64 = 0
	var read_response read_response_param_t
	var copied int

	/* this is the buffer we're going to pass to ioctl when we respond to this read, I make it
	here, to avoid at least or or two copies of the data. the storage interface will be given this buffer, and
	a position to write into. I suppose I could just pass it the slice to write into... */
	/* to figure out the slice, I must (sigh) serialize the zosbd2_operation structure to get the position of
	   userspacedatabuffer, but at least we don't have to copy or serialize all the data.
		 good news, the main run function already did this and passed it in */

	ret = storage.Read_block(entire_start, entire_length, kernelbuffer[this.m_sizeof_op:]) // so much for our data to kernelbuffer helper
	if ret != nil {
		this.m_log.Error("Error from storage.read_block, error: ", ret.Get_errmsg(), ", ", ret.Get_errcode())
		responseop.Error = int32(ret.Get_errcode()) // we have to return a successful read response with an error reported in it.
		// we have to return a successful read response with an error reported in it.
		goto error
	}

	/* and since read_block puts the read data into the buffer we give it, which is the buffer going back to the kernel
	 * we're pretty much done. */

	total_buffer_size_tally += uint64(entire_length)

	responseop.Header.Size = total_buffer_size_tally // the amount of data in userspacedatabuffer that we return, we tally this up as we go along through the bio segments. which we don't actually do anymore

	read_response.Response_total_length_of_all_segment_requests = total_buffer_size_tally
	// now copy read_response into responseop
	/* we don't want buffer to steal packet out of our struct, so we copy the contents, this may be unneccessary */
	buf = &bytes.Buffer{}
	err = binary.Write(buf, binary.LittleEndian, read_response)
	if err != nil {
		/* if this fails, then we can't send the error code back to the kernel, so we just bail. */
		return tools.ErrorWithCode(this.m_log, int(syscall.EINVAL), "unable to encode read response into response operation structure")
	}
	// now copy this into responseop.packet
	copied = copy(responseop.Packet[:], buf.Bytes())
	if copied != buf.Len() { // read_response is the shorter of the two, it has no padding
		/* if this fails, then we can't send the error code back to the kernel, so we just bail. */
		return tools.ErrorWithCode(this.m_log, int(syscall.EINVAL), "unable to copy packet into read response object packet")
	}

error:
	/* so we need the serialized responseop struction at the front of the main buffer we want to pass to the ioctl.
	   there's no way to make the serializer use my buffer, it requires a Writer so it can expand at will, and
	   the whole reason we went through this exercise was so that the serializer won't copy my meg of data.
	   so the simplest solution is to let it serialize to a new buffer and then just copy that 4152 serialized
	   bit to the front of my buffer and there we go. better we copy 4k than one meg. I'd rather copy nothing
	   but this is what I get for using a garbage collected language. */

	ret = this.optokernelbuffer(responseop, kernelbuffer)
	if ret != nil {
		/* if we can't make the thing we want to send to the kernel with an error code, we have to bail out of the read attempt */
		return tools.ErrorWithCode(this.m_log, ret.Get_errcode(), "Unable to serialize read response operation buffer to kernelbuffer: ", ret.Get_errmsg())
	}
	// now kernelbuffer is exactly what we want to return to the kernel...
	ret = this.Safe_ioctl(fd, IOCTL_DEVICE_OPERATION, kernelbuffer) // this causes caller to call block for request
	// get back the next request or error if the kernel sent it
	var ret2 = this.kernelbuffertoop(requestop, kernelbuffer)
	if ret2 != nil {
		return tools.ErrorWithCode(this.m_log, ret2.Get_errcode(), "unable to deserialize zosbd2 operation struct response after syscall:", ret2.Get_errmsg())
	}

	// see if the actual ioctl worked (might have an error in error field that we just deserialized )
	if ret != nil {
		return tools.ErrorWithCode(this.m_log, ret2.Get_errcode(), "ioctl call failed for device operation read response, err:", ret.Get_errmsg(),
			" kernel error code if any: ", requestop.Error)
	}
	// if we're here, the read response went in, we sent the data, blocked,and got the next request in requestop

	return nil // we blocked and got the next request.
}

func (this *Kmod) Respond_to_write_request(fd *os.File, handle_id uint32, requestop *zosbd2_operation, kernelbuffer []byte,
	storage zosbd2interfaces.Storage_mechanism) tools.Ret {

	/* same as with read, call the storage mechanism to do the write, it is responsible for read-update-write
	   and call the ioctl back with if it worked or not. */
	// 1/14/2021 use the grouping into one big write one.

	var ret = this.kernelbuffertoop(requestop, kernelbuffer)
	if ret != nil {
		return tools.ErrorWithCode(this.m_log, ret.Get_errcode(), "unable to get read request from kernelbuffer: ", ret.Get_errmsg())
	}

	var operation_id uint32 = requestop.Operation_id

	// get write_request out of operation struct
	var packetspace []byte = make([]byte, zosbd2_packet_size_bytes)
	copy(packetspace, requestop.Packet[:])
	var buf *bytes.Buffer = bytes.NewBuffer(packetspace)

	var write_request write_request_param_t
	var err = binary.Read(buf, binary.LittleEndian, &write_request)
	if err != nil {
		return tools.ErrorWithCode(this.m_log, -int(syscall.EINVAL), "invalid write request, unable to parse write request structure")
	}

	ret = this.Validate_write_request(handle_id, write_request) // make sure we're not going off the end of the disk, things like that.
	if ret != nil {
		return ret
	}

	var number_of_segments = write_request.Number_of_segments
	//        u64 total_length_of_all_bio_segments = requestop.packet.write_request.total_length_of_all_bio_segments; // in bytes, this is how much stuff is in the userspacedatabuffer that we're going to write out, verify below... xxxz

	// we have to return a response object with the ack, we took everything we're possibly going to write over, so we can re-use the memory, metadata doesn't get overwritten so we can still read from it
	var responseop *zosbd2_operation = requestop // response with the same pre-allocated operation struct, this isn't quite true for go, but it doesn't matter
	responseop.Handle_id = handle_id
	responseop.Operation_id = operation_id // pass this back in as is.
	responseop.Error = 0                   // default good unless overridden by error later
	responseop.Header.Operation = DEVICE_OPERATION_WRITE_RESPONSE
	responseop.Header.Size = 0 // the amount of data in data_buffer, write returns nothing.

	var entire_start uint64 = 0
	var entire_length uint32 = 0
	var adjret tools.Ret

	adjret, entire_start, entire_length = this.validate_adjacency(requestop, number_of_segments)
	if adjret != nil {
		return tools.ErrorWithCode(this.m_log, -int(syscall.EINVAL), "respond_to_write_request: ", adjret.Get_errmsg())
	}

	var total_buffer_size_tally uint64 = 0
	var write_response write_response_param_t
	var copied int

	total_buffer_size_tally += uint64(entire_length)

	// xxxz check and make sure we're not going past total_length_of_all_bio_segments, that's the max size of the available buffer to read stuff from.

	ret = storage.Write_block(entire_start, entire_length, kernelbuffer[this.m_sizeof_op:]) // so much for datatokernel helper
	if ret != nil {
		this.m_log.Error("Error from storage.write_block, error: ", ret.Get_errmsg(), ", ", ret.Get_errcode())
		responseop.Error = int32(ret.Get_errcode())
		// we have to return a successful write response with an error reported in it.
		goto error
	}

	// nothing to do for each segment after write completes, it either all worked or it didn't.
	responseop.Header.Size = 0 // for write this is still zero, we're not returning anything in the buffer.

	write_response.Response_total_length_of_all_segment_requests_written = total_buffer_size_tally // how much we processed so kmod can validate it if they want
	// now shove this write_response in the responseop
	buf = &bytes.Buffer{}
	err = binary.Write(buf, binary.LittleEndian, write_response)
	if err != nil {
		/* if this fails, then we can't send the error code back to the kernel, so we just bail. */
		return tools.ErrorWithCode(this.m_log, int(syscall.EINVAL), "unable to encode write response into response operation structure")
	}
	// now copy this into responseop.packet
	copied = copy(responseop.Packet[:], buf.Bytes())
	if copied != buf.Len() { // write_response is the shorter of the two, it has no padding
		/* if this fails, then we can't send the error code back to the kernel, so we just bail. */
		return tools.ErrorWithCode(this.m_log, int(syscall.EINVAL), "unable to copy packet into write response object packet")
	}

error:

	ret = this.optokernelbuffer(responseop, kernelbuffer)
	if ret != nil {
		/* if we can't make the thing we want to send to the kernel with an error code, we have to bail out of the write  attempt */
		return tools.ErrorWithCode(this.m_log, ret.Get_errcode(), "Unable to serialize write response operation buffer to kernelbuffer: ", ret.Get_errmsg())
	}
	// now kernelbuffer is exactly what we want to return to the kernel...
	ret = this.Safe_ioctl(fd, IOCTL_DEVICE_OPERATION, kernelbuffer) // this causes caller to block for request.

	// get back the next request or error if the kernel sent it
	var ret2 = this.kernelbuffertoop(requestop, kernelbuffer)
	if ret2 != nil {
		return tools.ErrorWithCode(this.m_log, ret2.Get_errcode(), "unable to deserialize zosbd2 operation struct response after syscall:", ret2.Get_errmsg())
	}

	// see if the actual ioctl worked (might have an error in error field that we just deserialized )
	if ret != nil {
		return tools.ErrorWithCode(this.m_log, ret.Get_errcode(), "ioctl call failed for device operation write response, err ", ret.Get_errcode(),
			" kernel error code if any: ", requestop.Error)
	}
	return nil // we blocked and got the next request.
}

func (this *Kmod) respond_to_discard_request(fd *os.File, handle_id uint32, requestop *zosbd2_operation, kernelbuffer []byte,
	storage zosbd2interfaces.Storage_mechanism) tools.Ret {
	/* similar to write, but noteworthy for a few points.
	 * we are not going to read-update write, we just need to send the discard request down the line.
	 * however, we still have to do the block range checking, because if this request covers a partial
	 * block, we do NOT discard that one, we just skip it. This goes for the block at the beginning and
	 * end of the range, which means we may end up doing nothing.
	 * now the kmod should never send us requests to discard anything unaligned, but also recall that our
	 * zos object block sizes can be different than the kmod 4k block size. But I think they have to be
	 * multiples, so it should never really come up.
	 * someday we may allow unmarking from the zos command line for objects, at which point it will come up
	 * which then brings up the question, if a partial block is unmarked, do we set the unmarked part of the
	 * block to zeroes? That's a read update write, which we can handle by calling write with zeroes, but
	 * for now since it should never happen, we'll just skip it. only call unmark, and only for whole blocks. */
	/* oh, I just realized, this is zos, we just send the request to the java program and it deals with all that. */
	// 1/14/2021 use the grouping into one big write one.

	var ret = this.kernelbuffertoop(requestop, kernelbuffer)
	if ret != nil {
		return tools.ErrorWithCode(this.m_log, ret.Get_errcode(), "unable to get discard request from kernelbuffer: ", ret.Get_errmsg())
	}

	var operation_id uint32 = requestop.Operation_id

	var packetspace []byte = make([]byte, zosbd2_packet_size_bytes)
	copy(packetspace, requestop.Packet[:])
	var buf *bytes.Buffer = bytes.NewBuffer(packetspace)

	var discard_request discard_request_param_t
	var err = binary.Read(buf, binary.LittleEndian, &discard_request)
	if err != nil {
		return tools.ErrorWithCode(this.m_log, -int(syscall.EINVAL), "invalid discard request, unable to parse discard request structure")
	}

	ret = this.Validate_discard_request(handle_id, discard_request) // make sure we're not going off the end of the disk, things like that.
	if ret != nil {
		return ret
	}

	var number_of_segments = discard_request.Number_of_segments

	// we have to return a response object with the ack, we took everything we're possibly going to write over, so we can re-use the memory, metadata doesn't get overwritten so we can still read from it
	var responseop *zosbd2_operation = requestop // response with the same pre-allocated operation struct, this isn't quite true for go, but it doesn't matter
	responseop.Handle_id = handle_id
	responseop.Operation_id = operation_id // pass this back in as is.
	responseop.Error = 0
	responseop.Header.Operation = DEVICE_OPERATION_DISCARD_RESPONSE // xxxz hmmm... kernel should verify we responded to what it asked for.
	responseop.Header.Size = 0                                      // the amount of data in data_buffer, discard returns nothing.

	ret = nil // default good.

	// 1/2/2021 as with read and write, make sure the discard request is one big contiguous block.

	var entire_start uint64 = 0
	var entire_length uint32 = 0
	var adjret tools.Ret

	/* the discard request will only ever come in as one segment, but we should do this anyway. */
	adjret, entire_start, entire_length = this.validate_adjacency(requestop, number_of_segments)
	if adjret != nil {
		return tools.ErrorWithCode(this.m_log, -int(syscall.EINVAL), "respond_to_discard_request: ", adjret.Get_errmsg())
	}

	var total_buffer_size_tally uint64 = 0
	var discard_response discard_response_param_t
	var copied int

	total_buffer_size_tally += uint64(entire_length)

	ret = storage.Discard_block(entire_start, entire_length)
	if ret != nil {
		ret = tools.ErrorWithCode(this.m_log, ret.Get_errcode(), "Error from storage.discard_block, error: ", ret.Get_errmsg(), ", ", ret.Get_errcode())
		responseop.Error = int32(ret.Get_errcode())
		// we have to return a successful discard response with an error reported in it.
		goto error
	}

	// nothing to do for each segment after discard completes, it either all worked or it didn't.
	responseop.Header.Size = 0 // for discard this is still zero, we're not returning anything in the buffer.

	discard_response.Response_total_length_of_all_segment_requests_discarded = total_buffer_size_tally // how much we processed so kmod can validate it if they want
	// now shove this discard_response in the responseop
	buf = &bytes.Buffer{}
	err = binary.Write(buf, binary.LittleEndian, discard_response)
	if err != nil {
		ret = tools.ErrorWithCode(this.m_log, -int(syscall.EINVAL), "unable to encode discard response into response operation structure")
		responseop.Error = int32(ret.Get_errcode())
		goto error
	}
	// now copy this into responseop.packet
	copied = copy(responseop.Packet[:], buf.Bytes())
	if copied != buf.Len() { // discard is the shorter of the two, it has no padding
		ret = tools.ErrorWithCode(this.m_log, -int(syscall.EINVAL), "unable to copy packet into discard response object packet")
		responseop.Error = int32(ret.Get_errcode())
		goto error
	}

error:

	ret = this.optokernelbuffer(responseop, kernelbuffer)
	if ret != nil {
		/* if we can't make the thing we want to send to the kernel with an error code, we have to bail out of the write  attempt */
		return tools.ErrorWithCode(this.m_log, ret.Get_errcode(), "Unable to serialize discard response operation buffer to kernelbuffer: ", ret.Get_errmsg())
	}

	ret = this.Safe_ioctl(fd, IOCTL_DEVICE_OPERATION, kernelbuffer) // this causes caller to block for request.

	// get back the next request or error if the kernel sent it
	var ret2 = this.kernelbuffertoop(requestop, kernelbuffer)
	if ret2 != nil {
		return tools.ErrorWithCode(this.m_log, ret2.Get_errcode(), "unable to deserialize zosbd2 operation struct response after syscall:", ret2.Get_errmsg())
	}

	// see if the actual ioctl worked (might have an error in error field that we just deserialized )
	if ret != nil {
		return tools.ErrorWithCode(this.m_log, ret.Get_errcode(), "ioctl call failed for device operation discard response, err ", ret.Get_errcode(),
			" kernel error code if any: ", requestop.Error)
	}
	return nil // we blocked and got the next request.
}
