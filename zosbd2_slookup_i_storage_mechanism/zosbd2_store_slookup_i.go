// SPDX-License-Identifier: LGPL-2.1
// Copyright (C) 2021-2022 stu mark

/* Package zosbd2_slookup_i_storage_mechanism exists to make the lint warning go away

this class implements the zosbd2 storage mechanism interface for slookup_i




which looks like this


type Storage_mechanism interface {
	Read_block(start_in_bytes uint64, length uint32, data []byte) tools.Ret

	Write_block(start_in_bytes uint64, length uint32, data []byte) tools.Ret

	Discard_block(start_in_bytes uint64, length uint32) tools.Ret

	// this is the zosobject block size, not the kernel block size.
	Get_block_size() uint32
}


so it goes

local block device -> storage mechanism (slookup_i) -> backingstore (file/block device/etc)

slookup is a bit simpler than stree_v. there are no keys, it's just a block number.

*/
package zosbd2_slookup_i_storage_mechanism

import (
	"container/list"
	"encoding/binary"

	"github.com/nixomose/nixomosegotools/tools"
	"github.com/nixomose/slookup_i_interfaces"
	"github.com/nixomose/zosbd2goclient/zosbd2cmdlib/zosbd2interfaces"
)

type Zosbd2_slookup_i_storage_mechanism struct {
	log           *tools.Nixomosetools_logger
	rawstore      *slookup_i_lib/slookup_i // the zosbd2cmd backing store is an slookup_i that was already injected with a file/memory backing store
	data_pipeline *list.List
}

var _ zosbd2interfaces.Storage_mechanism = &Zosbd2_slookup_i_storage_mechanism{}
var _ zosbd2interfaces.Storage_mechanism = (*Zosbd2_slookup_i_storage_mechanism)(nil)

func New_zosbd2_storage_mechanism(log *tools.Nixomosetools_logger, slookup_i *slookup_i_lib.slookup_i,
	data_pipeline *list.List) *Zosbd2_slookup_i_storage_mechanism {
	var z Zosbd2_slookup_i_storage_mechanism
	z.log = log
	z.rawstore = slookup_i
	z.data_pipeline = data_pipeline
	return &z
}


func (this *Zosbd2_slookup_i_storage_mechanism) read_one_block(block_num uint32, writebuffer []byte,
	buffer_size uint32) tools.Ret {
	// read this block from the backing store, if it's not there, provide zeros in the buffer
	// buffer_size is the number of bytes of data we should return
	var r, data = this.rawstore.Read(block_num) // read currentblock into writebuffer
	if r != nil {
		return r
	}

	/* we read data from the disk, run it through the pipeline */
	for item := this.data_pipeline.Front(); item != nil; item = item.Next() {
		// we should do this check once at startup not on each write, since the list doesn't change.
		var itemval = item.Value
		var pipline_element, ok = itemval.(zosbd2interfaces.Data_pipeline_element)
		// in go you must check for nil before casting to the list entry's type for some reason or it will panic
		if ok && pipline_element != nil {
			/* these all update in place, so they must resize the writebuffer to the actual size of the data
			after it is mutated */
			var ret = pipline_element.Pipe_out(&data)
			if ret != nil {
				return ret
			}
		} else {
			return tools.Error(this.log, "pipeline includes an element that isn't a data pipeline: ", itemval)
		}
	} // for

	// so much copying, sigh
	var copied = copy(writebuffer, data)
	if copied != len(writebuffer) {
		return tools.Error(this.log, "unable to copy entire block for read block, got ", copied, " of ", len(writebuffer))
	}

	if len(writebuffer) != int(buffer_size) {
		return tools.Error(this.log, "error reading block: ", block_key,
			", the final length of the block: ", len(writebuffer), " doesn't match the expected block size: ",
			buffer_size)
	}

	return nil
}

xxxxzs got up to here.

func (this *Zosbd2_slookup_i_storage_mechanism) write_one_block(block_num uint32, writebuffer []byte,
	buffer_size uint32) tools.Ret {
	// write this block to the backing store, 
		// buffer_size is the number of bytes of data we should write from the incoming buffer

	if len(writebuffer) != int(buffer_size) {
		return tools.Error(this.log, "invalid buffer length passed, buffer is: ", len(writebuffer), " but buffer size is: ", buffer_size)
	}

	/* run it through the pipeline */
	for item := this.data_pipeline.Front(); item != nil; item = item.Next() {
		// we should do this check once at startup not on each write, since the list doesn't change.
		var itemval = item.Value
		var pipline_element, ok = itemval.(zosbd2interfaces.Data_pipeline_element)
		// in go you must check for nil before casting to the list entry's type for some reason or it will panic
		if ok && pipline_element != nil {
			/* these all update in place, so they must resize the writebuffer to the actual size of the data to write */
			var ret = pipline_element.Pipe_in(&writebuffer)
			if ret != nil {
				return ret
			}
		} else {
			return tools.Error(this.log, "pipeline includes an element that isn't a data pipline: ", itemval)
		}
	}

	var r = this.rawstore.Write(block_num, writebuffer)
	if r != nil {
		return r
	}

	return nil
}

/* These functions have to break the large read into slookup_i sized chunks and read or write them all */

func (this *Zosbd2_slookup_i_storage_mechanism) Read_block(start_in_bytes uint64, length uint32, data []byte) tools.Ret {
	/* This function will perform a read of any byte position and length, spanning block boundaries if necessary.
	 * it will break up the possibly unaligned request into block aligned, block sized requests, and send back
	 * the correct results of what the caller asked for. */

	var slookup_i_block_size = this.Get_block_size()
	var currentblock uint64 = start_in_bytes / uint64(slookup_i_block_size) // this is the first block we need to read partial or not.
	var offsetinblock uint32 = uint32(uint64(start_in_bytes) % uint64(slookup_i_block_size))
	/* for updating metadata to the new length of the object, remember if this is a file and not a block device
	* objects can be any byte length, so we have to extend the object size to exactly the last byte written. */
	// var endwritepos uint64 = start_in_bytes + uint64(length)

	if len(data) < int(length) {
		return tools.Error(this.log, "Invalid read request, not enough storage supplied to read ", length, " bytes.")
	}

	// if length > stree_block_size {
	// 	z.log.Debug("we're going to loop through multiple blocks to read ", length, " bytes")
	// }

	/* just like write, they ask for pos+len, we return len bytes, by reading the partial first block if applicable
	* and keep reading blocks until we satify the request. */

	var howmuchread uint32 = 0
	var dataoutcopypos uint32 = 0 // this is a running counter of where we copy to in the caller's buffer
	for howmuchread < length {
		var readbuffer = make([]byte, slookup_i_block_size)
		var ret = this.read_one_block(currentblock, readbuffer, slookup_i_block_size) // or zeroes if it doesn't exist.
		if ret != nil {
			return ret
		}

		var start uint32 = offsetinblock                // for first block, start in middle if need be
		var end uint32 = start + (length - howmuchread) // absolute end position in buffer
		if end > stree_block_size {
			end = stree_block_size
		}
		var amounttoread uint32 = end - start

		// copy this part of this block to the caller's buffer as appropriate
		var readpos uint32 = start
		var copied = copy(data[dataoutcopypos:dataoutcopypos+amounttoread], readbuffer[readpos:])
		if copied != int(amounttoread) {
			return tools.Error(this.log, "copying block portion didn't copy entire portion, only copied ", copied, " of ", amounttoread)
		}

		readpos += amounttoread
		dataoutcopypos += amounttoread

		currentblock++
		offsetinblock = 0 // start next block at beginning
		howmuchread += amounttoread
	} // end for

	return nil

}

func (this *Zosbd2_slookup_i_storage_mechanism) Write_block(start_in_bytes uint64, length uint32, data []byte) tools.Ret {
	// swiped from objectstore.java

	/* So the four pieces are:
	 * 1) piece fits within a block but is not the whole block (starts after beginning, finishes before end)
	 * 2) piece starts in the middle, goes to the end
	 * 3) piece covers entire block
	 * 4) piece start at the beginning of a block, ends in the middle of a different block.
	 * For 1, 2 and 4, we have to do a read to get the existing block to update it. */

	var stree_block_size = this.Get_block_size()
	var currentblock uint64 = start_in_bytes / uint64(slookup_i_block_size)
	var offsetinblock uint32 = uint32(uint64(start_in_bytes) % uint64(slookup_i_block_size))
	/* for updating metadata to the new length of the object, remember if this is a file and not a block device
	 * objects can be any byte length, so we have to extend the object size to exactly the last byte written. */

	// if length > stree_block_size {
	// 	z.log.Debug("we're going to loop through multiple blocks to write ", length, " bytes")
	// }

	var written uint32 = 0 // we're given a buffer to write, can't be more than 2 gig, unlike discard which can be 2 gig.
	var readpos uint32 = 0 // where in the buffer we're reading from, can't be more than 1 meg let alone 2 gig
	for written < length {
		var block_key = Generate_key_from_block_num(currentblock)
		var remainingtowrite uint32 = length - written
		var absoluteendwritepositionrelativetothisblock uint32 = offsetinblock + remainingtowrite

		var prefetch bool = false
		if offsetinblock > 0 { // case 1 and 2
			prefetch = true
		}
		if absoluteendwritepositionrelativetothisblock < stree_block_size {
			prefetch = true // case 1 and 4
		}

		var writebuffer []byte = make([]byte, slookup_i_block_size)

		if prefetch {
			// z.log.Debug("reading block ", currentblock, " length ", stree_block_size,
			// 	" to update part of a block pos ", offsetinblock, " len ", remainingtowrite)
			var r = this.read_one_block(block_key, writebuffer, slookup_i_block_size)
			if r != nil {
				return r
			}
		} //else {
		//  writebuffer already filled with zeros
		// Arrays.fill(wb, (byte)0); // zero out from previous block
		// writebufferholder.add(wb);
		// }

		/* 3/19/2022 for really large lengths (like almost 2^32) end will overflow and cause problems.
		this never really happens for writes, but it does for discard. */
		var start uint32 = offsetinblock            // the start of where to write in this block's buffer
		var end uint32 = start + (length - written) // the absolute end position in this buffer
		if end > stree_block_size {
			end = stree_block_size
		}

		var amounttowrite uint32 = end - start

		// fill up the write buffer as appropriate

		var copied = uint32(copy(writebuffer[start:end], data[readpos:readpos+amounttowrite]))
		if copied != (end - start) {
			this.log.Error("we didn't copy all the data we wanted to. expected: ", end-start,
				" copied: ", copied)
		}
		readpos += amounttowrite

		var r = this.write_one_block(block_key, writebuffer, slookup_i_block_size)
		if r != nil {
			return r
		}

		currentblock++
		offsetinblock = 0 // start next block at beginning
		written += amounttowrite
	} // end for

	// z.log.Debug("finished write_new for pos: ", ((start_in_bytes + uint64(length)) - uint64(length)), ", len: ", length)
	return nil

}

func (this *Zosbd2_slookup_i_storage_mechanism) Discard_block(start_in_bytes uint64, length uint32) tools.Ret {
	/* Find all the blocks completely encompassed by this range (partial blocks are skipped) and
	 * delete them.
	 * If this unmap gets to the end of the file, lower the last write position/eof position. */
	/* 2/6/2021 no, don't do that, object size stays the same, the truncate command can resize if
	 * that's what they want to do. */
	/* delete the localblocks row for any whole-block discard
	   for any partial block, we need to do a read update with zeros write, but we'll skip
	   that for now, since the kernel will never come in with those.
	   actually that's not true, our block sizes aren't the same size so they may not line
	   up with the discard request that comes in because it can cover a vast swath of disk. */
	this.log.Debug("got discard request for start: ", start_in_bytes, " length: ", length)

	var stree_block_size = this.Get_block_size()

	var currentblock uint64 = start_in_bytes / uint64(slookup_i_block_size)
	var offsetinblock uint32 = uint32(uint64(start_in_bytes) % uint64(slookup_i_block_size))
	var endwritepos uint64 = start_in_bytes + uint64(length)

	if length == 0 {
		return nil // we moved the 'well that was easy' to here because we need validate to create the object on a zero length write
	}

	/* So the four pieces are:
	* 1) piece fits within a block but is not the whole block (starts after beginning, finishes before end)
	* 2) piece starts in the middle, goes to the end
	* 3) piece covers entire block
	* 4) piece start at the beginning of a block, ends in the middle of a different block.
	* For 1, 2 and 4, we have to do a read to get the existing block to update it. */

	/* if we do end up updating blocks (someday) to zero out a partially discarded block,
	 * this is the time to use to say that this is the write time of all the localblocks in
	 * this transaction and the object write date */
	/* 2/6/2021 later in the day, so since write which we copied already has all the plumbing for
	 * read update write, we'll do it now here too.
	 * We stole most of this from write, which does almost everything we need and it's all tested
	 * and working, so this should be pretty easy. */
	/* 3/19/2021 discard ranges come in as really large, 32 bit large, ints don't cut it. */

	// do all the whole block discards first, when that's all worked out, update the object metadata
	var written int64 = 0
	var readpos int64 = 0 // where in the buffer we're reading from, can't be more than 1 meg let alone 2 gig, this is for padding spaces around unaligned discards

	// this is what we write over the partiablly discarded block
	var zeroes []byte = make([]byte, slookup_i_block_size)

	var discard_range_start int64 = -1
	var discard_range_end int64 = -1
	// consider the case where these never get set because we're discarding something tiny
	for written < int64(length) {
		var block_key = Generate_key_from_block_num(currentblock)
		var remainingtowrite uint64 = uint64(length) - uint64(written)
		var absoluteendwritepositionrelativetothisblock uint64 = uint64(offsetinblock) + remainingtowrite // xxxzoverflow

		var prefetch bool = false
		if offsetinblock > 0 { // case 1 and 2
			prefetch = true
		}
		if absoluteendwritepositionrelativetothisblock < uint64(stree_block_size) {
			prefetch = true // case 1 and 4
		}

		var writebuffer []byte = make([]byte, slookup_i_block_size)
		if prefetch {
			var remainingtowriteinthisblock = uint64(slookup_i_block_size) - uint64(offsetinblock)
			if remainingtowriteinthisblock > absoluteendwritepositionrelativetothisblock {
				remainingtowriteinthisblock = absoluteendwritepositionrelativetothisblock
			}
			// we have a partial block, we need to zero out some end part of it.
			this.log.Debug("for discard, reading block ", currentblock, " length ", stree_block_size,
				" to update part of a block pos ", offsetinblock, " len ", remainingtowriteinthisblock)
			var r = this.read_one_block(block_key, writebuffer, slookup_i_block_size)
			if r != nil {
				return r
			}
		} // else {
		// 	//  writebuffer already filled with zeros
		// 	// writebufferholder.add(zeros);
		// }

		var start uint64 = uint64(offsetinblock)                   // the start of where to write in this block's buffer
		var end uint64 = start + uint64((int64(length) - written)) // the absolute end position in this buffer, 3/19/2022 for discard of really large almost 2^32 lengths this overflows
		if end > uint64(stree_block_size) {
			end = uint64(stree_block_size)
		}
		var amounttowrite int64 = int64(end) - int64(start)

		/* only if this is a prefetch do we have to write anything, otherwise we just discard
		* the localblocks row. */
		if prefetch {
			var copied = uint64(copy(writebuffer[start:end], zeroes[0:amounttowrite]))
			if copied != (end - start) {
				this.log.Error("we didn't copy all the zeros we wanted to. expected: ", end-start,
					" copied: ", copied)
			}
			readpos += amounttowrite

			var r = this.write_one_block(block_key, writebuffer, slookup_i_block_size) // update half block on disk
			if r != nil {
				return r
			}

		} else {
			/* just discard the block. this takes too long, so let's just gather the range
			* and do it in one shot at the end. */
			if discard_range_start == -1 {
				discard_range_start = int64(currentblock)
			}
			discard_range_end = int64(currentblock)
		}
		currentblock++
		offsetinblock = 0 // start next block at beginning
		written += amounttowrite
	} // end for

	this.log.Debug("discarding block range ", discard_range_start, " to ", discard_range_end, " inclusive.")
	if discard_range_start == -1 || discard_range_end == -1 {
		this.log.Debug("nothing to discard.")
	} else {
		/* funny that we never actually called delete...
		   So the zos version of this that I got this from was able to to a sweeping delete in one
			 call, but slookup_i doesn't have that so we just loop through the blocks anyway. maybe
			 someday there will be a one shot way of doing this... */
		for currentblock = uint64(discard_range_start); currentblock <= uint64(discard_range_end); currentblock++ {
			var block_key = Generate_key_from_block_num(currentblock)
			this.log.Debug("discard deleting block: ", currentblock)
			this.rawstore.Delete(block_key, false) // not found is not an error here.
		}
		this.log.Debug("finished discard for pos: ", (endwritepos - uint64(length)), ", len: ", length)
	}
	return nil
}

// Get_block_size this is the zosobject block size, not the kernel block size.
func (this *Zosbd2_slookup_i_storage_mechanism) Get_block_size() uint32 {
	/* this is actually not the stree_node_value size, but the size one tree element
	can hold which is the value_size * (additional nodes per block + 1) */
	var block_size = this.rawstore.Get_node_size_in_bytes()
	return block_size
}
