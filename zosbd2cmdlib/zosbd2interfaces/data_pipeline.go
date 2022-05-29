// Package zosbd2interfaces has a package comment
package zosbd2interfaces

import (
	"github.com/nixomose/nixomosegotools/tools"
	"github.com/spf13/cobra"
)

type Data_pipeline_element interface {

	/* 12/26/2021 this is the interface that allows one to dynamically add
	compression or encryption or any other kind of data mutation as it goes through the system. */

	Process_parameters(params *cobra.Command) tools.Ret
	Process_device(device Device_interface) tools.Ret

	Pipe_in(data_in_out *[]byte) tools.Ret  // compress/encrypt
	Pipe_out(data_in_out *[]byte) tools.Ret // decompress/decrypt

	Get_context() Data_pipeline_element_context
	Set_context(Data_pipeline_element_context)
}

type Data_pipeline_element_context interface {

	/* 4/16/2022 this is the interface that lets you extend a context for an instance of a
	particular pipeline. So if you need to keep state for a compression pipeline, this is how
	you get access to it. */

	Create() tools.Ret

	Get_context() Data_pipeline_element_context
}
