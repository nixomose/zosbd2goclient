// SPDX-License-Identifier: LGPL-2.1
// Copyright (C) 2021-2022 stu mark

package zosbd2cmdlib

/* The IOCTL command list for the kernel module */
/* 1/25/2020 changing all the defines to u64s (for ioctl) and u32 for functions so the bindgen thing in rust
 * will come up with the correct u64 types, and not the u32 types it comes up with from #defines.
 * The problem though is we can't use #define for switch case, so we're leaving the OPERATION constants
 * as defines with a cast to hopefully drive the issue home. */

/* in 2016, theodore ts'o said that the max block size is 4k. And That's That.
   https://stackoverflow.com/questions/30585565/bypassing-4kb-block-size-limitation-on-block-layer-device
   my followup:
   https://stackoverflow.com/questions/61805646/is-there-a-way-to-make-the-kernel-treat-a-block-device-like-it-has-a-larger-than
*/

/* 12/24/2021 this is a port of zosbdshare.h, not an automated c translation like the rust version.
   I couldn't get cgo to work easily and cleanly so I just translated everything from c to go.
	 as a result everything here must match zosbd2share.h exactly in size and shape with the kernel
	 module version or nothing's going to work right. So if you make a change there, you have to make
	 a change here. */

// const kmod_block_size int = 4096 // all zosbd2 blocks are 4k. period.

/* system wide constants */

const MAX_DEVICE_NAME_LENGTH int = (DISK_NAME_LEN - 1) // from genhd.h, -1 for nt.

const CREATE_DEVICE_MIN_NUMBER_BLOCK_COUNT int = 1024 // can't have less than this number of kernel_block_size blocks

// apparently bindgen has a problem with this. it still makes this a u32

const CREATE_DEVICE_MAX_NUMBER_BLOCK_COUNT uint64 = 3221225472 // blocks have to be at least 4k, so this is 12tb in 4k blocks, ~3.2 billion blocks

const CREATE_DEVICE_MIN_TIMEOUT_SECONDS int = 1
const CREATE_DEVICE_MAX_TIMEOUT_SECONDS int = 1200 // 10 minute max is absurd, but is handy for testing

const CREATE_DEVICE_MAX_ACTIVE_BLOCK_DEVICES int = 50 // we won't try to manage more than 50 devices at one time.

/* the kernel seems to tend towards 32 a lot of times, so that's what I'm setting this to get as much
 * parallelism out of it as much of the time as possible. There may be more to tune. */
/* 8/24/2020 it seems that mkfs.ext2 will in fact send 256 4k segments (1 meg) in one shot, so let's use 256 */
// const MAX_BIO_SEGMENTS_PER_REQUEST 32 // the maximum number of bio segments the kernel side will send to userspace in one request

const MAX_BIO_SEGMENTS_PER_REQUEST = 256                                                   // the maximum number of bio segments the kernel side will send to userspace in one request
const MAX_BIO_SEGMENTS_BUFFER_SIZE_PER_REQUEST int = (MAX_BIO_SEGMENTS_PER_REQUEST * 4096) // the maximum buffer size of data for multiple segments we will ship to userspace in one go. This assumes 4k blocks will which not be true forever, but it is for now.

/* zosbd2 control device ioctls */

const IOCTL_CONTROL_PING = 12                 // ping the driver make sure it's here.
const IOCTL_CONTROL_CREATE_DEVICE = 21        // create a new block device
const IOCTL_CONTROL_DESTROY_DEVICE_BY_ID = 22 // destroy an existing block device.
const IOCTL_CONTROL_DESTROY_DEVICE_BY_NAME = 23
const IOCTL_CONTROL_DESTROY_ALL_DEVICES = 25
const IOCTL_CONTROL_GET_HANDLE_ID_BY_NAME = 26

const IOCTL_CONTROL_STATUS_DEVICE_LIST = 60   // return the list of device handle_ids
const IOCTL_CONTROL_STATUS_DEVICE_STATUS = 61 // return state of a single device

/* zosbd2 block device ioctls */

// there's one ioctl command for all block device operations going to userspace.
// the packet encodes what kind of operation it is.

const IOCTL_DEVICE_PING = 53 // ping the device make sure it's here.

const IOCTL_DEVICE_OPERATION = 58 // the main way to interact with the device for read/write(/trim, etc)

/* zosbd2 operations from userspace to kernel (part 2) */

const DEVICE_OPERATION_NO_RESPONSE_BLOCK_FOR_REQUEST = 30 // the first time in we haven't read any data, we're just there to block
const DEVICE_OPERATION_READ_RESPONSE = 31                 // all other calls we are responding with data we read
const DEVICE_OPERATION_WRITE_RESPONSE = 37                // all other calls we are responding with data we wrote and return success or failure.
const DEVICE_OPERATION_DISCARD_RESPONSE = 38              // all other calls we are responding with blocks discarded and return success or failure.

const DEVICE_OPERATION_STATUS = 39 // all other calls we are responding with data we wrote and return success or failure.

/* zosbd2 operations from kernel to userspace. (part 1) */

const DEVICE_OPERATION_KERNEL_BLOCK_FOR_REQUEST = 40 // only used in error cases to ask userspace to call us back to block
const DEVICE_OPERATION_KERNEL_READ_REQUEST = 41
const DEVICE_OPERATION_KERNEL_WRITE_REQUEST = 42
const DEVICE_OPERATION_KERNEL_DISCARD_REQUEST = 43

const DEVICE_OPERATION_KERNEL_USERSPACE_EXIT = 8086 // tell userspace to stop calling us, we're going away.

/*  this is how I figured out the padding:
#include "zosbd2share.h"

control_block_device_create_params x1;
control_block_device_destroy_by_id_params x2;
control_block_device_destroy_by_name_params x3;
control_block_device_destroy_all_params x4;
control_block_device_get_handle_id_by_name_params x5;
control_ping x06;
control_status x07;
control_operation x08;
read_request_param_t x09;
read_response_param_t x10;
write_request_param_t x11;
write_response_param_t x12;
discard_request_param_t x13;
discard_response_param_t x14;
zosbd2_packet x15;
zosbd2_header x16;
zosbd2_bio_segment_metadata x17;
zosbd2_operation x18;

clang -c layouts.c -Xclang -fdump-record-layouts  > layouts.txt
*/

type control_block_device_create_params struct {
	/* Params passed in from userspace needed to create block device */
	Device_name            [MAX_DEVICE_NAME_LENGTH + 1]byte // the name of the device in /dev, +1 for nt.
	Kernel_block_size      u32                              // kinda has to be 4k or less because of kernel limitations.
	Padding1               u32                              // to match the C header
	Number_of_blocks       u64                              // this times kernel_block_size is the the size of the device in bytes
	Device_timeout_seconds u32                              // how long the block device will wait for userspace to respond to a request before throwing an io error on the block device.
	// Padding2                     u32                              // to match the C header
	Max_segments_per_request u32 // how many segments the kmod will pass to userspace per request.
	// Padding3                     u32                              // to match the C header
	Bio_segment_batch_queue_size u32 // 1/18/2021 how many write bio segments we will queue up before processing, 0 = off

	// we return these to userspace.
	Handle_id u32
	Error     s32
}

type control_block_device_destroy_by_id_params struct {
	/* Params passed in from userspace needed to destroy a block device by id */
	Handle_id u32
	Error     s32
	Force     u32
}

type control_block_device_destroy_by_name_params struct {
	/* Params passed in from userspace needed to destroy a block device by name */
	Device_name [MAX_DEVICE_NAME_LENGTH + 1]byte // the name of the device in /dev, +1 for nt.
	Error       s32
	Force       u32
}

type control_block_device_destroy_all_params struct {
	/* Params passed in from userspace needed to destroy all block devices, so we can pass 'force' */
	Force u32
}

// type control_block_device_get_handle_id_by_name_params struct {
// 	/* Params passed in from userspace needed to get a block device handle_id by name */
// 	Device_name [MAX_DEVICE_NAME_LENGTH + 1]byte // the name of the device in /dev, +1 for nt.
// 	Handle_id   u32                              // returned to userspace
// 	Error       s32
// }

type control_ping struct {
	// no data for ping
}

type control_status_device_list_request_response struct {
	// returns a count of devices and an array of device handle_ids
	// what device's handle_id to get status for
	Num_devices u32
	Handleid    [CREATE_DEVICE_MAX_ACTIVE_BLOCK_DEVICES]u32
}

type control_status_device_request_response struct {
	// returns status for a single device
	Handle_id_request u32
	Padding1          u32 // to match the C header
	// the rest is response fields
	Size                     u64                              // size of the block device in bytes
	Number_of_blocks         u64                              // this times kernel_block_size is the total device size in bytes.
	Kernel_block_size        u32                              // saving for reference, we don't really need to keep this, this is 4k
	Max_segments_per_request u32                              // how many segments the kmod should return to userspace per request.
	Timeout_milliseconds     u32                              // number of milliseconds before the block device gives up waiting for userspace to respond. kernel uses ms, converted from seconds on the way in
	Handle_id                u32                              // so they can check what we gave them is what they asked for
	Device_name              [MAX_DEVICE_NAME_LENGTH + 1]byte // the name of the device in /dev, +1 for nt.
}

/* below we have the problem of a union being a member of a struct, here it is the top level struct
   so userspace, can just use the appropriate control_* struct directly. */
// type  union control_operation {
//   control_block_device_create_params create_params
//   control_block_device_destroy_by_id_params destroy_params_by_id
//   control_block_device_destroy_by_name_params destroy_params_by_name
//   control_block_device_destroy_all_params destroy_params_all
//   control_block_device_get_handle_id_by_name_params get_handle_id_params_by_name
//   control_ping ping_params
//   control_status status_params
// }

type read_request_param_t struct {
	// read request param details are now in op_state list for each segment
	Total_length_of_all_bio_segments u64 // how much to read in bytes total
	Number_of_segments               u32 // in this request
}

type read_response_param_t struct {
	Response_total_length_of_all_segment_requests u64 // how much data userspace is returning in buf[0] it total
}

type write_request_param_t struct {
	// 4/9/2020 write request details are now in the op_state list for each segment
	Total_length_of_all_bio_segments u64 // how much to write in total
	Number_of_segments               u32 // in this request
	// op->size should match, referring to the amount of data in databuffer, this is actually redundant, we'll remove it later.
}

type write_response_param_t struct {
	Response_total_length_of_all_segment_requests_written u64 // how much data userspace actually wrote
}

type discard_request_param_t struct {
	Total_length_of_all_bio_segments u64
	Number_of_segments               u32
}

type discard_response_param_t struct {
	Response_total_length_of_all_segment_requests_discarded u64
}

/* rust has unions solely to support this kind of thing, go does not. so we just hardcode the
   size of the largest packet struct so it will line up with the C struct */
/* that adds up to 12 but the zosbd2_operation struct is 8 byte aligned and 12 isn't
   divisible by 8 so we're going to add 4 bytes for padding at the end.
	 the layouts dump says sizeof zosbd2_packet is 16 not 12. 16 it is. */

const zosbd2_packet_size_bytes int = 16 // u64+u32 = 12 bytes, possibly +4 for padding, but I doubt it. indeed I was wrong, we needed it after all.
// type  union zosbd2_packet{
//   read_request_param_t read_request
//   read_response_param_t read_response
//   // writes
//   write_request_param_t write_request
//   write_response_param_t write_response
//   // discards
//   discard_request_param_t discard_request
//   discard_response_param_t discard_response
// }

type zosbd2_header struct {
	Operation u32 // what kind of data is being passed in this ioctl
	Padding   u32 // pad out to 8 bytes
	Size      u64 // the size of the databuffer to move between userspace and kernelspace, this will be the sum of all the lengths of all of the segments in the request.
	// for writes, this amount needs to be copied to userspace, for reads, this amount needs to be copied to the kernel
	Signal_number u32 // any ioctl call can be interrupted by a signal. If it is, this value is set to the signal number.
}

type zosbd2_bio_segment_metadata struct {
	Start  u64
	Length u64 // this only need be a u32 but it will get padded to this anyway. favoring 64 bit.
}

type packet_type = [zosbd2_packet_size_bytes]byte

// everything in here is a fixed sized array so it always serializes/deserializes to/from the same exact C structure
type zosbd2_operation struct {
	Handle_id u32 // userspace must provide this so we can know which device it's requesting io commands for.
	Padding1  u32 // structures must be 64 bit aligned I guess?
	Header    zosbd2_header
	Padding2  u32         // to get packet to align to 8
	Packet    packet_type // size of largest packet struct (in C this is a union, so here it's just an array)
	Metadata  [MAX_BIO_SEGMENTS_PER_REQUEST]zosbd2_bio_segment_metadata

	Error        s32 // zero is okay, non-zero means the userspace side reported an error to the block device thread side.
	Operation_id u32 // the unique_id for this operation (created at request time) so we can match the response from userspace to the request.

	// the databuffer must be the last thing in the operation structure.
	/* We used to use this as a holding area for bio segment buffer data when going from kernel to userspace,
	 * but as it turns out, we never did that with the read, we only did it with the write
	 * and there was no point, we can save ourselves a copy operation by having userspace
	 * read and write directly from the bio segment buffer. we still have to copy that buffer over the blood
	 * brain barrier copy_to_user, but that's only one copy and we can't avoid it, we can avoid this one. */
	// lining this up on a page boundary would probably be a win.
	Userspacedatabuffer [0]byte // this is a placeholder since the block size is defined by the user at device create time
	//userspacedatabuffer [256 * 4096]byte // this is a placeholder since the block size is defined by the user at device create time

}

/* other constants, from ZOSDefs.java */

const MAX_DISK_GROUP_LENGTH int = 64 // these are zen level things, there's probably some limit in zendemic already.
const MAX_OBJECTID_LENGTH int = 1024 // just like s3
