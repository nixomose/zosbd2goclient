#!/bin/sh

# MAKE SURE YOU CHMOD 755 this script or it will silently fall back and not use it.
echo "start dlv-sudo" >> /tmp/d1
echo "whoami: `whoami`" >> /tmp/d1

if ! which dlv ; then
	PATH="${GOPATH}/bin:$PATH"
  echo "add path $PATH" >> /tmp/d1
fi
if [ "$DEBUG_AS_ROOT" = "true" ]; then
#  echo "debug as root" >> /tmp/d1
	DLV=$(which dlv)
  DLV="/home/nixo/go/bin/dlv-dap"
  DLV="/home/nixo/go/bin/dlv"

	echo "DIV=$DLV"  >> /tmp/d1
	echo exec /usr/bin/sudo "$DLV" --only-same-user=false "$@" >> /tmp/d1
       exec /usr/bin/sudo "$DLV" --only-same-user=false "$@"
 #      exec sudo "$DLV"  --only-same-user=false "$@"


else
#  echo "not debug as root" >> /tmp/d1
	exec "/home/nixo/go/bin/dlv" "$@"
fi
#echo "exiting" >> /tmp/d1
