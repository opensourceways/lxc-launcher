#!/bin/bash
# You will used three files at the same time: lxd.config,cert.crt,init.sh; These files will be deteled auto when script executed completed ;
# Mail:"zhaochunjiang1@huawei.com";
# Created by 2021-12-17;

echo "#############################################################################################"
# check files
if [ ! -e ./cert.crt ] || [ ! -e ./lxd-container.config ] || [ ! -e ./lxd-vm.config ];then
	echo "miss init files"
    exit
fi

# yum update and install
##yum clean all && yum update -y && yum makecache
if hash snap 2>/dev/null;then
	yum remove snapd -y 
fi
yum install snapd -y
systemctl enable --now snapd.socket
ln -s /var/lib/snapd/snap /snap

echo "#############################################################################################"
# lxd install
echo "plaese wait,installing lxd........."
sleep 2
if ! hash lxd 2>/dev/null;then
	snap install lxd > /dev/null
	if [ $? -ne 0 ];then
		echo "Install faild this time,will install again ,please waiting ........."
		while :
		do
			# "snap install lxd --channel=4.20/stable > /dev/null" had missed;lxd version-4.20 
			# had been removed,you shoud install latest stable either not design channel=xx; 
			# default lxd latest stable verison is 4.23 at 2022-02-28;you also use 
			# "snap install lxd --channel=latest/stable > /dev/null" ;
			snap install lxd 
			if [ $? -ne 0 ];then
				sleep 1
				continue
			else
				echo "snap install succeeded"
				break
			fi
		done
	fi
fi
export PATH="$PATH:/snap/bin"
/usr/bin/env | grep -w '/snap/bin'
if [ $? -ne 0 ];then
        echo "snap enviroment not effection"
        exit
fi

echo "#############################################################################################"
# lxd config
str=`ip add | grep "eth0" | tail -1 | awk '{print $2}'`
sed -i "s#core.https_address:.*#core.https_address: ${str%/*}:8443#g" ./lxd.config
## you shoud replace "/dev/vdc" with the pool device name, and modify the device name in lxd.config to keep up with;
if [ -e /dev/vdc ];then
        wipefs -f -a /dev/vdc
fi
if [ `lxc storage list | wc -l` -gt 3 ];then
	echo "lxc default pool is : default; the default pool will be delete"	
	lxc storage delete default
fi
if [ `lshw -class system | grep -i virtual` -eq 0 ];then
	echo "virtual arch building"
	str="container"
	lxd init --preseed < ./lxd-container.config
else
	echo "machine arch building"
	lxd init --preseed < ./lxd-vm.config
	lxc profile set default security.secureboot=false
fi
if [ $? -ne 0 ];then
        echo "lxd init faild"
        exit
fi
echo "lxd init succeeded"

lxc config trust add internal ./cert.crt
if [ $? -ne 0 ];then
        echo "lxc cert import faild"
        exit
fi
echo "lxc cert import succeeded"

echo "#############################################################################################"
ebtables -F
# iptables rules
ebtables -N ip_isolate
ebtables -N input_secure
ebtables -P ip_isolate DROP
ebtables -A FORWARD -p ip -i veth+ -j ip_isolate
ebtables -I INPUT 1 -p ip -i veth+ -j input_secure
ebtables -A ip_isolate -p ip -i veth+ --ip-src 10.205.172.1/24 --ip-dst 10.205.172.1 -j ACCEPT
ebtables -A ip_isolate -p ip -i veth+ --ip-src 10.205.172.1 --ip-dst 10.205.172.1/24 -j ACCEPT
ebtables -A ip_isolate -p ip -i veth+ --ip-src 10.205.172.1/24 --ip-dst 10.205.172.1/24 -j DROP
ebtables -A input_secure -j RETURN
if [ $str = "container" ];then
	ebtables -A input_secure -p ipv4 -i veth+ --ip-dst 169.254.0.0/16 --ip-proto tcp -j DROP
fi
# arp rules
ebtables -N flood_secure
ebtables -P flood_secure DROP
ebtables -I FORWARD 1 -d ff:ff:ff:ff:ff:ff -j flood_secure
ebtables -A flood_secure -s 00:16:3e:a1:a2:a3 -i lxdbr0 -d ff:ff:ff:ff:ff:ff -j ACCEPT
ebtables -A flood_secure -p ARP -i lxdbr0 -j ACCEPT
ebtables -A flood_secure -p IPv4 -i ! lxdbr0 --ip-proto udp -j DROP
ebtables -A flood_secure -s 00:16:3e:00:00:00/00:16:3e:ff:ff:ff -i ! lxdbr0 -d ff:ff:ff:ff:ff:ff -j DROP
ebtables-save > /etc/sysconfig/ebtables
if [[ -z `grep -w "/usr/bin/cat" /etc/rc.d/rc.local` ]];then
        echo -e '/usr/bin/cat /etc/sysconfig/ebtables | ebtables-restore' >> /etc/rc.d/rc.local
        chmod +x /etc/rc.d/rc.local
fi

echo "#############################################################################################"
# check lxc config
lxc info
rm -rf ../../{lxc-launcher,init.sh}
systemctl disable sshd 
systemctl stop sshd
systemctl stop systemd-tmpfiles-clean.timer
systemctl disable systemd-tmpfiles-clean.timer
echo "init finish,system will be reboot...,you can use 【crtl+c】 cancel reboot in 5 seconds"
sleep 5
/usr/sbin/reboot
