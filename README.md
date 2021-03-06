# EC2FakeImds

Note: This should no longer be possible in most AWS accounts. A few months after this talk (https://blog.ryanjarv.sh/2020/12/04/deja-vu-in-the-cloud.html) AWS rolled out a change that prevents you from overriding the 169.254.169.254/32 route if you have never used this functionality in the past.

## Warning
This code may break things in strange ways.

## Setup

```
% terraform apply
% cd fake_imds_route
% make init
```

Launch an instance in any zone accept the one used for the fake IMDS server (us-east-1f unless changed) and it should come up with the /msg file created by the fake metadata server.

## Debugging

If it's taking a really long time to boot up the instance (more then a few minutes), and the routes arn't reverting, something in the user data on the IMDS instance is likely breaking. You can wait 20 minutes or so and the instance should come up, at that point you can test the user-data being served by the Fake IMDS server by running `cloud-init clean` and `cloud-init init`. You'll have to log in to the IMDS server though to edit it since it pulls it from their each time.

## Alternatives

If you are able to change instance attributes for an instance the  [UserDataSwap](https://github.com/RyanJarv/UserDataSwap) is likely a better choice. It does the same thing effectively. The main reason for this PoC is to prove you can serve your own user data without any permission's to the victim EC2 instance. In the future I'd like to see if I can limit permissions to only what's in AWS's network admin policy.

There are a few other benefits to this approach as well but they are mostly outweighed by complexity of the set up (compared to [UserDataSwap](https://github.com/RyanJarv/UserDataSwap)) and the fact you need to have full permissions to at least one node in the VPC. There are likely alternaties to the last requirement, so in general I'd simply recommend monitoring for any route changes involving 169.254.169.254/32 (see the [overview](#Overview) section more info).

## Overview

For a very high level overview of the issue I'm attempting to demonstrate here you can see my blog post on [AWS IMDS Persistence/Priv Escalation](https://blog.ryanjarv.sh/2020/10/19/imds-persistence.html)

The backstory of this though is I made a very hacky PoC for this in one of my accounts. It works well enough but everything was hardcoded and only would work in a single VPC in my account. So this repo is an attempt to make that PoC a bit more generic so that anyone can dig into it and get a better idea of how it works.

If you are looking for a way to detect this kind of activity you can check out the [NonDefaultMetadataServer](https://github.com/RyanJarv/awsconfig#nondefaultmetadataserverg) config check I made for this. It hasn't had extensive use yet, but it seems to work well enough in my testing (just remember to set up an alert after configuring it).

## Details

Since this is still a work in progress there some glue tying a few of the things together right now. Instead of a howto, I'll just go over each of the parts as much as I can for now.

### fake_imds_route

SAM lambda function that runs when a the RunInstance API is called. It should be fast enough (my previous version was at least) to switch out the routes before the new instance tries to contact the fake IMDS server. We don't handle reverting since that is done through a script when served to the new instance from the fake IMDS server.

Originally I simply added and removed a hardcoded 169.254.169.254/32, this script attempts to work in any setup by making a back up of the route table(s) that the new instance uses as well as the fake IMDS server, attaching the former to the subnet the new instance is running in and injecting a static route of 169.254.169.254/32 pointing to the fake IMDS server.

Thinking about this again though I suppose I only need to do this for the new instance, not the IMDS server as well. The main thing is the IMDS server's route isn't affected, however we ignore any RunInstance commands in the same subnet of the fake IMDS server for this reason.

### User Data

This is actually served as apart of the Fake IMDS Instance nginx root directory. There's enough interesting stuff in the [user-data](https://github.com/RyanJarv/EC2FakeImds/blob/main/files/imds/latest/user-data) for it to warrent it's own section though.

When we get to the point that the victims node is executing the user-data we can [Revert the routes](https://github.com/RyanJarv/EC2FakeImds/blob/main/files/imds/latest/user-data#L9) and then [re-init cloud-init and reboot](https://github.com/RyanJarv/EC2FakeImds/blob/main/files/imds/latest/user-data#L39).

Of course our malicous script is executed (as root) somewhere [at the top](https://github.com/RyanJarv/EC2FakeImds/blob/main/files/imds/latest/user-data#L4).


### Terraform

This just spins up a instance to act as the fake IMDS server. This server has source/dest checking disabled since the static route we inject is essentially treating this instance as a gateway. It also run's a start up script which I go over in the [Fake IMDS Instance](#Fake-IMDS-Instance) section.

### Fake IMDS Instance

You can find the nginx config and web directory under [./imds](https://github.com/RyanJarv/EC2FakeImds/tree/main/files/imds). This is deployed to the instance using terraform provisioners.

This is a strange setup but it seems to work so far.

First we have nginx rewriting any first part of the path to /latest since cloud-init sometimes want's specific versions, so we do this to prevent maintaining multiple directory structures of the same config.

```
rewrite ^/(.+?)/(.+)$ /latest/$2 last;
```

Next we attempt to serve from the [imds folder](https://github.com/RyanJarv/EC2FakeImds/tree/main/files/imds). You'll notice that the directory is almost entirely blank files, we'll go into that a bit more below.

```
root /var/www/imds;
try_files $uri @custom $uri/index @proxy;
```

If that fails we try @custom.. which apparently doesn't exist anymore so.. not sure what I was doing there. Anyways, next the index, and finally the upstream IMDS proxy, which is our connection to the attacker node's real IMDS.

```
upstream imds {
        server 169.254.169.254;
}
```

So we have our own IMDS serving *mock* responses when it's not caught first by the filesystem. We do this is because cloud-init need's to get far enough in the cycle that it executes our custom user-data. We just send our semi-bogus responses hoping it doesn't screw anything up. In the case it does, we attempt to stop cloud-init from enumerating those resources by responding with nothing (i.e the blank files mentioned earlier).

The paths we need to block are mostly relate to networking, they don't seem necessary for our purposes in the boot process and everything will be re-inited with the right IMDS server later anyways. FYI the re-init process triggered in our [user-data script](https://github.com/RyanJarv/EC2FakeImds/blob/main/files/imds/latest/user-data#L39) just after we set the routes back to default in the same script.

All this would of course would be unacceptable in any real application.. just opening up your real IMDS to the world (or vpc in our case), but in reality an attacker isn't really going to care about that kind of thing. It's not really their data they are losing (or maybe it is now.. lol).

The [user data](https://github.com/RyanJarv/EC2FakeImds/blob/main/files/imds/latest/user-data) script actually just uses fake IMDS's instance profile (cred requests are proxied to the real IMDS in our nginx config).

We also do some interesting stuff with iptables in the fake IMDS [startup script](https://github.com/RyanJarv/EC2FakeImds/blob/scripts/nginx-imds.conf) in order to get routing to work here. We serve traffic sent to us intended to 169.254.169.254 (remember we're behaving as the next hop in the routing table) by listening on 169.254.168.254 (third octect 168 vs 169), while still being able to use the nodes real IMDS like normal. This is essential for mocking out the parts of the IMDS service we don't care about.

So we add the IP the nginx server should listen on:
```
ip addr add 169.254.168.254/32 dev eth0
```

Short circuit outbound routing to the IMDS ip in the NAT table. This allows us to talk to our real IMDS server. (NOTE: Just realized I need to unhardcode the instances private IP here, or figure out a better way to do this).
```
iptables -t nat -A PREROUTING -s 169.254.168.254,172.31.54.46 -d 169.254.169.254/32 -j RETURN
```

All other traffic to the IMDS IP is DNAT'd to our nginx server listening on `169.254.168.254`.
```
iptables -t nat -A PREROUTING -d 169.254.169.254/32 -j DNAT --to-destination 169.254.168.254
```

Then we MASQUERADE outbound traffic coming from nginx listening on 169.254.168.254. This is needed for our nginx server to make requests to our real IMDS, 169.254.168.254 isn't really routable to us, either that or the real IMDS server doesn't like seeing traffic coming from it.
```
iptables -t nat -A POSTROUTING -s 169.254.168.254 -d 169.254.169.254/32 -o eth0 -j MASQUERADE
```

Thinking over some of the iptables stuff here, I'm thinking it could be much simpler.. I guess this is just the first thing that worked for me so I kept it, something like that.
 

