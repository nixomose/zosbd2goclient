package storage

import (
	"syscall"

	"github.com/nixomose/nixomosegotools/tools"
	"github.com/nixomose/zosbd2goclient/zosbd2cmdlib/zosbd2interfaces"
)

var _ zosbd2interfaces.Storage_mechanism = &Ramdiskstorage{}
var _ zosbd2interfaces.Storage_mechanism = (*Ramdiskstorage)(nil)

type Ramdiskstorage struct {
	/* For testing the client code without the backend zos client we make a simple ramdisk */

	m_log *tools.Nixomosetools_logger
	// this is the zosobject block size, not the kernel block size.
	m_block_size uint32
	ramdisk      map[uint64][]byte
}

func New_ramdiskstorage(log *tools.Nixomosetools_logger, block_size uint32) *Ramdiskstorage {
	var ret Ramdiskstorage
	ret.m_log = log
	ret.m_block_size = block_size
	ret.ramdisk = make(map[uint64][]byte)
	return &ret
}

func (this *Ramdiskstorage) Get_block_size() uint32 {
	return this.m_block_size
}

func (this *Ramdiskstorage) Read_block(start_in_bytes uint64, length uint32, dataout []byte) tools.Ret {
	/* first we'll test this side and make a not so simple ramdisk here, and then we'll test the back side where
	 * the java stuff will implement a ramdisk.
	 * the thing here is that we have to break up the request into 4k chunks because any of them can be
	 * requested at any time, so we can't just write what we're given as one big block. */

	var start_copy_location = 0
	for length > 0 {

		var found bool
		var data []byte
		data, found = this.ramdisk[start_in_bytes]

		if found == false {
			// return a bunch of zeroes, 4k I mean.
			data = make([]byte, this.m_block_size)
		}

		// copy it right into the caller's buffer at the right spot
		this.m_log.Debug("ramdisk read from ", start_in_bytes, " to ", start_in_bytes+uint64(this.m_block_size))
		var copied int = copy(dataout[start_copy_location:start_copy_location+int(this.m_block_size)], data)
		if copied != int(this.m_block_size) {
			return tools.ErrorWithCode(this.m_log, int(syscall.ENODATA), "unable to copy data from ramdisk, only copied: ", copied)
		}

		start_in_bytes += uint64(this.m_block_size)
		start_copy_location += int(this.m_block_size)
		length -= this.m_block_size
	}

	return nil
}

func (this *Ramdiskstorage) Write_block(start_in_bytes uint64, length uint32, data []byte) tools.Ret {
	// break up the write into 4k and write each one to the map
	var start_copy_location = 0
	for length > 0 {
		var d = make([]byte, 4096)
		this.m_log.Debug("ramdisk write to ", start_in_bytes, " to ", start_in_bytes+4096)
		var copied int = copy(d, data[start_copy_location:start_copy_location+4096])
		if copied != 4096 {
			return tools.ErrorWithCode(this.m_log, int(syscall.ENODATA), "unable to copy data to write to ramdisk, only copied: ", copied)
		}
		this.ramdisk[start_in_bytes] = d
		start_in_bytes += 4096
		start_copy_location += 4096
		length -= 4096
	}
	return nil
}

func (this *Ramdiskstorage) Discard_block(start_in_bytes uint64, length uint32) tools.Ret {
	return tools.ErrorWithCode(this.m_log, -int(syscall.EINVAL), "trim not supported yet.")
}
