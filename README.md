# Notes
This project consists of two components
1. lxc instance launcher
2. lxc instance management
# LXC-launcher
We can't integrate lxc/lxd into kubernetes due to the Image and Runtime spec difference,
this project acts as a lxc instance agent in kubernetes which responsible for lxc instance lifecycle management as well
as the network proxy.
# LXC manager
This component used for lxc image download, extract and load it into lxd server for the purpose of efficiency, the image
will be in the format of:
```Dockerfile
FROM scratch
ADD lxd.tar.xz /
ADD rootfs.squashfs /
```
Also, the unmanaged lxc instance will be GCed during maintenance.

# Requirement
1. lxd server to be interactive with.
2. socat package use for lxc instance network proxy.

# Limits
lxc-launcher communicates to the lxd server and its lxc instance via socket files, therefore, two mounted socket files
are required.

# TODO
1. Use IP and Port to visit lxd server instead of local domain socket.
2. Validate iSula works well on Ubuntu 20.04.

# Install

# Build and Test
