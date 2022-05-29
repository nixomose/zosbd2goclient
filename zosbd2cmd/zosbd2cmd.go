package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/nixomose/nixomosegotools/tools"
	"github.com/nixomose/zosbd2goclient/zosbd2cmd/storage"
	"github.com/nixomose/zosbd2goclient/zosbd2cmdlib"
)

const BLOCK_SIZE = 4096
const TIMEOUT_IN_SECONDS = 1200
const control_device = "/dev/zosbd2ctl"

func main() {
	var params = os.Args[1:]
	if len(params) < 3 {
		fmt.Println("zosbd2cmd -d <device name> -s <size> -b <backing device/file>")
		return
	}

	var (
		device_name_flag    = flag.String("d", "", "device name")
		size_flag           = flag.Uint64("s", 1048576, "size in bytes must be multiple of 4k")
		storage_device_flag = flag.String("b", "", "backing device/file")
	)
	flag.Parse()

	var device_name = *device_name_flag
	var size = *size_flag
	var storage_device = *storage_device_flag

	var log = tools.New_Nixomosetools_logger(tools.DEBUG)
	log.Debug("device name: ", device_name)
	log.Debug("size: ", size)
	log.Debug("backing storage: ", storage_device)

	var number_of_blocks uint64 = size / uint64(BLOCK_SIZE)

	var storage = storage.New_ramdiskstorage(log, BLOCK_SIZE)

	var kmod = zosbd2cmdlib.New_kmod(log)

	// open the control device
	var ret, control_device_fd = kmod.Open_bd(control_device)
	if ret != nil {
		log.Error("Unable to create block device: ", device_name, " error: ", ret.Get_errmsg())
		return
	}
	defer kmod.Close_bd(control_device_fd)

	/* ping the driver */

	ret = kmod.Control_ping(control_device_fd)
	if ret != nil {
		log.Error("Unable to create block device: ", device_name, " error: ", ret.Get_errmsg())
		return
	}

	/* Create the block device */
	var handle_id uint32
	ret, handle_id = kmod.Create_block_device(control_device_fd, device_name, BLOCK_SIZE, number_of_blocks, TIMEOUT_IN_SECONDS)
	if ret != nil {
		log.Error("Unable to create block device: ", device_name, " error: ", ret.Get_errmsg())
		return
	}

	/* Destroy the block device by id */
	ret = kmod.Destroy_block_device_by_id(control_device_fd, handle_id)
	if ret != nil {
		log.Error("Unable to destroy block device by id: ", handle_id, " device: ", device_name, " error: ", ret.Get_errmsg())
		return
	}

	/* Create the block device */
	ret, handle_id = kmod.Create_block_device(control_device_fd, device_name, BLOCK_SIZE, number_of_blocks, TIMEOUT_IN_SECONDS)
	if ret != nil {
		log.Error("Unable to create block device: ", device_name, " error: ", ret.Get_errmsg())
		return
	}

	/* Destroy the block device by name */
	ret = kmod.Destroy_block_device_by_name(control_device_fd, device_name)
	if ret != nil {
		log.Error("Unable to destroy block device by name: ", device_name, " error: ", ret.Get_errmsg())
		return
	}

	/* Create 2 block devices */
	ret, handle_id = kmod.Create_block_device(control_device_fd, device_name, BLOCK_SIZE, number_of_blocks, TIMEOUT_IN_SECONDS)
	if ret != nil {
		log.Error("Unable to create block device: ", device_name, " error: ", ret.Get_errmsg())
		return
	}

	var device_name_2 = device_name + "2"
	ret, _ = kmod.Create_block_device(control_device_fd, device_name_2, BLOCK_SIZE, number_of_blocks, TIMEOUT_IN_SECONDS)
	if ret != nil {
		log.Error("Unable to create block device: ", device_name_2, " error: ", ret.Get_errmsg())
		return
	}

	/* Destroy all block devices */
	ret = kmod.Destroy_all_block_devices(control_device_fd)
	if ret != nil {
		log.Error("Unable to destroy all block_devices, error: ", ret.Get_errmsg())
		return
	}

	/* now test IO */

	/* Create block device */
	ret, handle_id = kmod.Create_block_device(control_device_fd, device_name, BLOCK_SIZE, number_of_blocks, TIMEOUT_IN_SECONDS)
	if ret != nil {
		log.Error("Unable to create block device: ", device_name, " error: ", ret.Get_errmsg())
		return
	}

	var block_device_handler = zosbd2cmdlib.New_block_device_handler(log, &kmod, device_name, storage, handle_id)

	defer func() {
		ret = kmod.Destroy_block_device_by_name(control_device_fd, device_name)

		if ret != nil {
			log.Error("Error destroying block device for: ", device_name, " error: ", ret.Get_errmsg())
		}
	}()

	/* go run the thing */
	ret = block_device_handler.Run()
	if ret != nil {
		log.Error("Error running block device handler for: ", device_name, " error: ", ret.Get_errmsg())
	}

}
