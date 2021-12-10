#!/bin/bash

echo "preparing container default profile"
lxc profile delete container
lxc profile create container
lxc profile edit container < ./container.yaml

echo "preparing virtual machine default profile"
lxc profile delete virtualmachine
lxc profile create virtualmachine
lxc profile edit virtualmachine < ./virtualmachine.yaml

echo "done"
