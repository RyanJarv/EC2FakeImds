# EC2FakeImds (WIP)

For more info see https://blog.ryanjarv.sh/2020/10/19/imds-persistence.html.

Originally I made a very hacky PoC for this in one of my accounts. It works well enough but everything was hardcoded and only would work in a single VPC in my account. This repo is an attempt to make that PoC a bit more generic so that anyone can dig into it and get a better idea of how it works.

If you are looking for a way to detect this kind of activity you can check out the [NonDefaultMetadataServer](https://github.com/RyanJarv/awsconfig#nondefaultmetadataserverg) config check I made for this. It hasn't had extensive use yet, but it seems to work well enough in my testing (just remember to set up an alert after configuring it).

## Setup

Since this is still a work in progress there some glue tying a few of the things together right now.

### fake_imds_route

SAM lambda function that runs when a the RunInstance API is called. It should be fast enough (my previous version was at least) to switch out the routes before the new instance tries to contact the fake IMDS server. We don't handle reverting since that is done through a script when served to the new instance from the fake IMDS server.

WIP Note: Currently you need to update the parameters in the sam config template to the appropriate values.

Originally I simply added and removed a hardcoded 169.254.169.254/32, this script attempts to work in any setup by making a back up of the route table(s) that the new instance uses as well as the fake IMDS server, attaching the former to the subnet the new instance is running in and injecting a static route of 169.254.169.254/32 pointing to the fake IMDS server. Thinking about this again though I suppose I only need to do this for the new instance, not the IMDS server as well. The main thing is the IMDS server's route isn't affected, however we ignore any RunInstance commands in the same subnet of the fake IMDS server for this reason.

### terraform

This just spins up a instance to act as the fake IMDS server. This server has source/dest checking disabled since the static route we inject is essentially treating this instance as a gateway.

### Fake IMDS Instance

Note: All this config is in the repo but not deployed to the instance currently. Needs to be manually copied if you want to test this at the moment. You can find the nginx config and web directory under [./imds](https://github.com/RyanJarv/EC2FakeImds/tree/main/imds)

This is a strange setup but it seems to work so far. First we have nginx rewriting any first part of the path to /latest since cloud-init sometimes want's specific versions, so we do this to prevent maintaining multiple directory structures of the same config.

```
rewrite ^/(.+?)/(.+)$ /latest/$2 last;
```

Next we attempt to serve from the imds folder.

```
root /var/www/imds;
try_files $uri @custom $uri/index @proxy;
```

If that fails we try @custom.. which apparently doesn't exist anymore so.. not sure what I was doing there. Anyways, next the index, and finally the upstream IMDS proxy.

```
upstream imds {
        server 169.254.169.254;
}
```

So we have our own IMDS serving *mock* (for lack of a better term) responses when it's not caught first by the filesystem. The main reason we do this is because we need cloud-init to get far enough in the cycle that it executes our custom user-data, so we just send our bogus responses hoping it doesn't screw anything up. We actually do have to block a few things and return empty responses, mostly relating to networking or else the victim's instance will get confused. If you see any blank index files in the directory structure, that's what's going on there.

This of course would be unacceptable in any real application to do something like this, but in reality an attacker isn't going to care that they are serving out keys or what not to anyone inside the VPC that asks.

Right now the [user data](https://github.com/RyanJarv/EC2FakeImds/blob/main/imds/latest/user-data) just uses the hardcoded keys, but if the instance had the right permissions we need here we could potentially just omit the creds there. The victim instance should be using the fake IMDS instance profile at this point in the boot process (should, haven't tested this).

We do some interesting stuff with iptables in the fake IMDS [startup script](https://github.com/RyanJarv/EC2FakeImds/blob/main/main.tf#L52) to get all this routing to work both ways. We serve traffic sent to us intended to 169.254.169.254 (remember we're behaving as the next hop in the routing table) by listening on 169.254.168.254 (third octect 168 vs 169), while still being able to use the nodes real IMDS like normal. This is essential for mocking out the parts of the IMDS service we don't care about.
 

