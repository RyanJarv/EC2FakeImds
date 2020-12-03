#!/usr/bin/env bash
set -euo pipefail

sudo apt-get update
sudo apt-get install -y nginx


# Needed so terraform can copy files
sudo chown ubuntu /var/www/
sudo chown ubuntu /etc/nginx/sites-available/default

# This is a dirty hack to allow us to serve traffic for 169.254.169.254 as well as use our own IMDS server at
# 169.254.169.254 over eth0. Note that we use 169.254.168.254 to for the nginx listen IP not 169.254.169.254 despite
# it appearing as the second from other hosts. i.e everyone but us get's the fake IMDS server.
#
# Reaching our own IMDS is important for normal functioning of the instance, but we also use it as a fallback in nginx
# to mock out values. This means that other nodes are accessing our real IMDS server in cases we didn't care to mock
# out in nginx.
sudo ip addr add 169.254.168.254/32 dev eth0
sudo iptables -t nat -A PREROUTING -s 169.254.168.254,172.31.54.46 -d 169.254.169.254/32 -j RETURN
sudo iptables -t nat -A PREROUTING -d 169.254.169.254/32 -j DNAT --to-destination 169.254.168.254
sudo iptables -t nat -A POSTROUTING -s 169.254.168.254 -d 169.254.169.254/32 -o eth0 -j MASQUERADE
