// SPDX-License-Identifier: LGPL-2.1
// Copyright (C) 2021-2022 stu mark

package zosbd2interfaces

import "github.com/nixomose/nixomosegotools/tools"

type Storage_mechanism interface {
	Read_block(start_in_bytes uint64, length uint32, data []byte) tools.Ret

	Write_block(start_in_bytes uint64, length uint32, data []byte) tools.Ret

	Discard_block(start_in_bytes uint64, length uint32) tools.Ret

	// this is the zosobject block size, not the kernel block size.
	Get_block_size() uint32
}
