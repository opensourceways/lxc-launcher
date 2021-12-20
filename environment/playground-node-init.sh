#!/bin/bash
# You will used three files at the same time: lxd.config,cert.crt,init.sh; These files will be deteled auto when script executed complete ;
# Maintance by CJ; mail:"zhaochunjiang1@huawei.com";Created by 2021-12-17;

# check files
if [ ! -e ./cert.crt ] && [ ! -e ./lxd.config ];then
        echo "miss init files"
        exit
fi

# yum update and install
yum clean all && yum update -y && yum makecache && yum install snapd -y
if [[ -z `rpm -qa | grep "snapd"` ]];then
        echo "install snapd faild"
        exit
fi
systemctl enable --now snapd.socket
ln -s /var/lib/snapd/snap /snap

# lxd install
while :
do
        echo "plaese wait,installing lxd........."
        sleep 10
        snap install lxd --channel=4.20/stable
        if [ $? -eq 0 ];then
                echo "snap install succeeded"
                break
        fi
done
export PATH="$PATH:/snap/bin"
/usr/bin/env | grep -w '/snap/bin'
if [ $? -ne 0 ];then
        echo "snap enviroment not effection"
        exit
fi

# lxd config
str=`ip add | grep "eth0" | tail -1 | awk '{print $2}'`
sed -i "s/ip_add/${str%/*}/g" ./lxd.config
if [ ! -e /dev/vdc ];then
        echo "miss data disk"
        exit
fi
lxd init --preseed < ./lxd.config
if [ $? -ne 0 ];then
        echo "lxd init faild"
        exit
else
        echo "lxd init succeeded"
fi

lxc config trust add internal ./cert.crt
if [ $? -ne 0 ];then
        echo "lxc cert import faild"
        exit
else
        echo "lxc cert import succeeded"
fi

# iptables rules
ebtables -N ip_isolate
ebtables -P ip_isolate DROP
ebtables -A FORWARD -p ip -i veth+ -j ip_isolate
ebtables -A ip_isolate -p ip -i veth+ --ip-src 10.205.172.1/24 --ip-dst 10.205.172.1 -j ACCEPT
ebtables -A ip_isolate -p ip -i veth+ --ip-src 10.205.172.1 --ip-dst 10.205.172.1/24 -j ACCEPT
ebtables -A ip_isolate -p ip -i veth+ --ip-src 10.205.172.1/24 --ip-dst 10.205.172.1/24 -j DROP
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
lxc info
systemctl stop sshd
systemctl disable sshd
rm -f ./cert.crt ./lxd.config ./init.sh
echo "init finish,system will be reboot..."
/usr/sbin/reboot
