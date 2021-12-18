#pragma once

/* The IOCTL command list for the kernel module */
/* 1/25/2020 changing all the defines to u64s (for ioctl) and u32 for functions so the bindgen thing in rust
 * will come up with the correct u64 types, and not the u32 types it comes up with from #defines.
 * The problem though is we can't use const for switch case, so we're leaving the OPERATION constants
 * as defines with a cast to hopefully drive the issue home. */


/* in 2016, theodore ts'o said that the max block size is 4k. And That's That.
   https://stackoverflow.com/questions/30585565/bypassing-4kb-block-size-limitation-on-block-layer-device
   my followup:
   https://stackoverflow.com/questions/61805646/is-there-a-way-to-make-the-kernel-treat-a-block-device-like-it-has-a-larger-than
*/

#define kmod_block_size 4096 // all zosbd2 blocks are 4k. period.


#include "zosbd2userkerneldef.h"


/* system wide constants */

#define MAX_DEVICE_NAME_LENGTH (DISK_NAME_LEN-1) // from genhd.h, -1 for nt.

#define CREATE_DEVICE_MIN_NUMBER_BLOCK_COUNT 1024 // can't have less than this number of kernel_block_size blocks
// apparently bindgen has a problem with this. it still makes this a u32
#define CREATE_DEVICE_MAX_NUMBER_BLOCK_COUNT 3221225472ULL // blocks have to be at least 4k, so this is 12tb in 4k blocks, ~3.2 billion blocks

#define CREATE_DEVICE_MIN_TIMEOUT_SECONDS 1
#define CREATE_DEVICE_MAX_TIMEOUT_SECONDS 1200 // 10 minute max is absurd, but is handy for testing

#define CREATE_DEVICE_MAX_ACTIVE_BLOCK_DEVICES 50 // we won't try to manage more than 50 devices at one time.

/* the kernel seems to tend towards 32 a lot of times, so that's what I'm setting this to get as much
 * parallelism out of it as much of the time as possible. There may be more to tune. */
/* 8/24/2020 it seems that mkfs.ext2 will in fact send 256 4k segments (1 meg) in one shot, so let's use 256 */
// #define MAX_BIO_SEGMENTS_PER_REQUEST 32 // the maximum number of bio segments the kernel side will send to userspace in one request
#define MAX_BIO_SEGMENTS_PER_REQUEST 256 // the maximum number of bio segments the kernel side will send to userspace in one request
#define MAX_BIO_SEGMENTS_BUFFER_SIZE_PER_REQUEST (MAX_BIO_SEGMENTS_PER_REQUEST * 4096) // the maximum buffer size of data for multiple segments we will ship to userspace in one go. This assumes 4k blocks will which not be true forever, but it is for now.


/* zosbd2 control device ioctls */

#define IOCTL_CONTROL_PING 12  // ping the driver make sure it's here.
#define IOCTL_CONTROL_STATUS 20 // return state of the driver and version and feature flags
#define IOCTL_CONTROL_CREATE_DEVICE 21 // create a new block device
#define IOCTL_CONTROL_DESTROY_DEVICE_BY_ID 22 // destroy an existing block device.
#define IOCTL_CONTROL_DESTROY_DEVICE_BY_NAME 23
#define IOCTL_CONTROL_DESTROY_ALL_DEVICES 25
#define IOCTL_CONTROL_GET_HANDLE_ID_BY_NAME 26


/* zosbd2 block device ioctls */

// there's one ioctl command for all block device operations going to userspace.
// the packet encodes what kind of operation it is.
#define IOCTL_DEVICE_PING 53  // ping the device make sure it's here.
#define IOCTL_DEVICE_STATUS 55  // get status from a particular block device

#define IOCTL_DEVICE_OPERATION 58 // the main way to interact with the device for read/write(/trim, etc)


/* zosbd2 operations from userspace to kernel (part 2) */

#define DEVICE_OPERATION_NO_RESPONSE_BLOCK_FOR_REQUEST 30    // the first time in we haven't read any data, we're just there to block
#define DEVICE_OPERATION_READ_RESPONSE 31        // all other calls we are responding with data we read
#define DEVICE_OPERATION_WRITE_RESPONSE 37       // all other calls we are responding with data we wrote and return success or failure.
#define DEVICE_OPERATION_DISCARD_RESPONSE 38     // all other calls we are responding with blocks discarded and return success or failure.

#define DEVICE_OPERATION_STATUS 39       // all other calls we are responding with data we wrote and return success or failure.


/* zosbd2 operations from kernel to userspace. (part 1) */
#define DEVICE_OPERATION_KERNEL_BLOCK_FOR_REQUEST 40 // only used in error cases to ask userspace to call us back to block
#define DEVICE_OPERATION_KERNEL_READ_REQUEST 41
#define DEVICE_OPERATION_KERNEL_WRITE_REQUEST 42
#define DEVICE_OPERATION_KERNEL_DISCARD_REQUEST 43

#define DEVICE_OPERATION_KERNEL_USERSPACE_EXIT 8086 // tell userspace to stop calling us, we're going away.


typedef struct control_block_device_create_params_t
  {
    /* Params passed in from userspace needed to create block device */
    unsigned char device_name[MAX_DEVICE_NAME_LENGTH+1]; // the name of the device in /dev, +1 for nt.
    u32 kernel_block_size; // kinda has to be 4k or less because of kernel limitations.
    u64 number_of_blocks; // this times kernel_block_size is the the size of the device in bytes
    u32 device_timeout_seconds; // how long the block device will wait for userspace to respond to a request before throwing an io error on the block device.
    u32 max_segments_per_request; // how many segments the kmod will pass to userspace per request.
    u32 bio_segment_batch_queue_size; // 1/18/2021 how many write bio segments we will queue up before processing, 0 = off

    // we return these to userspace.
    u32 handle_id;
    s32 error;
  } control_block_device_create_params;

typedef struct control_block_device_destroy_by_id_params_t
  {
    /* Params passed in from userspace needed to destroy a block device by id */
    u32 handle_id;
    s32 error;
    u32 force;
  } control_block_device_destroy_by_id_params;

typedef struct control_block_device_destroy_by_name_params_t
  {
    /* Params passed in from userspace needed to destroy a block device by name */
    unsigned char device_name[MAX_DEVICE_NAME_LENGTH+1]; // the name of the device in /dev, +1 for nt.
    s32 error;
    u32 force;
  } control_block_device_destroy_by_name_params;

typedef struct control_block_device_destroy_all_params_t
  {
    /* Params passed in from userspace needed to destroy all block devices, so we can pass 'force' */
    u32 force;
  } control_block_device_destroy_all_params;

typedef struct control_block_device_get_handle_id_by_name_params_t
  {
    /* Params passed in from userspace needed to get a block device handle_id by name */
    unsigned char device_name[MAX_DEVICE_NAME_LENGTH+1]; // the name of the device in /dev, +1 for nt.
    u32 handle_id; // returned to userspace
    s32 error;
  } control_block_device_get_handle_id_by_name_params;

typedef struct control_ping_t
  {
  // no data for ping
  } control_ping;

typedef struct control_status_t
  {
    // what device to get status for
    u32 all_devices; // if you set this to zero, it looks up the handle_id
    u32 handle_id;
  } control_status;


typedef union
  {
    control_block_device_create_params create_params;
    control_block_device_destroy_by_id_params destroy_params_by_id;
    control_block_device_destroy_by_name_params destroy_params_by_name;
    control_block_device_destroy_all_params destroy_params_all;
    control_block_device_get_handle_id_by_name_params get_handle_id_params_by_name;
    control_ping ping_params;
    control_status status_params;
  } control_operation;


typedef struct
{
  // read request param details are now in op_state list for each segment
  u64 total_length_of_all_bio_segments; // how much to read in bytes total
  u32 number_of_segments; // in this request
} read_request_param_t;

typedef struct
{
  u64 response_total_length_of_all_segment_requests; // how much data userspace is returning in buf[0] it total
} read_response_param_t;

typedef struct
{
    // 4/9/2020 write request details are now in the op_state list for each segment
  u64 total_length_of_all_bio_segments; // how much to write in total
  u32 number_of_segments; // in this request
  // op->size should match, referring to the amount of data in databuffer, this is actually redundant, we'll remove it later.
} write_request_param_t;

typedef struct
{
    u64 response_total_length_of_all_segment_requests_written; // how much data userspace actually wrote
} write_response_param_t;

typedef struct
{

  u64 total_length_of_all_bio_segments;
  u32 number_of_segments;

} discard_request_param_t;

typedef struct
{
    u64 response_total_length_of_all_segment_requests_discarded;
} discard_response_param_t;

typedef union
{
    read_request_param_t read_request;
    read_response_param_t read_response;
    // writes
    write_request_param_t write_request;
    write_response_param_t write_response;
    // discards
    discard_request_param_t discard_request;
    discard_response_param_t discard_response;
} zosbd2_packet;

typedef struct
{
    u32 operation; // what kind of data is being passed in this ioctl
    u64 size; // the size of the databuffer to move between userspace and kernelspace, this will be the sum of all the lengths of all of the segments in the request.
    // for writes, this amount needs to be copied to userspace, for reads, this amount needs to be copied to the kernel
    u32 signal_number; // any ioctl call can be interrupted by a signal. If it is, this value is set to the signal number.
} zosbd2_header;

typedef struct
{
    u64 start;
    u64 length; // this only need be a u32 but it will get padded to this anyway. favoring 64 bit.
} zosbd2_bio_segment_metadata;

typedef struct
{
    u32 handle_id; // userspace must provide this so we can know which device it's requesting io commands for.
    zosbd2_header header;
    zosbd2_packet packet;
    zosbd2_bio_segment_metadata metadata[MAX_BIO_SEGMENTS_PER_REQUEST];

    s32 error; // zero is okay, non-zero means the userspace side reported an error to the block device thread side.
    u32 operation_id; // the unique_id for this operation (created at request time) so we can match the response from userspace to the request.

    // the databuffer must be the last thing in the operation structure.
    /* We used to use this as a holding area for bio segment buffer data when going from kernel to userspace,
     * but as it turns out, we never did that with the read, we only did it with the write
     * and there was no point, we can save ourselves a copy operation by having userspace
     * read and write directly from the bio segment buffer. we still have to copy that buffer over the blood
     * brain barrier copy_to_user, but that's only one copy and we can't avoid it, we can avoid this one. */
    // lining this up on a page boundary would probably be a win.
    unsigned char userspacedatabuffer[0]; // this is a placeholder since the block size is defined by the user at device create time

} zosbd2_operation;




/* other constants, from ZOSDefs.java */
#define  MAX_DISK_GROUP_LENGTH  64 // these are zen level things, there's probably some limit in zendemic already.
#define  MAX_OBJECTID_LENGTH  1024 // just like s3

