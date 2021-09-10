# Lxc-launcher
We can't integrate lxc/lxd into kubernetes due to the Image and Runtime spec difference,
this project acts as a lxc instance agent in kubernetes which responsible for lxc instance lifecycle management as well
as the network proxy.

# Requirement
1. lxd server
2. socat package

# Limits
lxc-launcher communicates to the lxd server and its lxc instance via socket files, therefore, two mounted socket files
are required.

# Install

# Build and Test
