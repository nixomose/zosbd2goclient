package zosbd2cmdlib

/* this is a go translation of the zosbd2share.h header file.
   the structures must match exactly with their C equivalent or ioctls to the kernel module won't work.
   and it turns out import "C" can solve this problem for us. */

/*
#include "zosbd2share.h"
*/
import "C"
