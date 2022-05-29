// Package zosbd2interfaces has a package comment to make the linter happy
package zosbd2interfaces

type Device_interface interface {

	/* we need to pass an interface to the kompressor callbacks, and we can't actually pass
	a device because it would make a circular reference since device is defined in lbd_lib
	but we can make an interface for it. ahhh interfaces, the solution to everything... */
	/* so remember, a block is 64k with the filestore header it's like 65588 or something
	     but stree doesn't know or care about that. a stree node however is
			 65536 * additional_nodes + 1, and that is the max amount of data stree can store in an
			 stree node, which can then get compressed down to some number of 65536 blocks.
			 so when kompressor comes looking for the size of the max data in a storable block
			 it's really asking for a node not a block */

	Get_node_size_in_bytes() uint32
}
