# ReverseProxy

Reverse proxy subservice for ArozOS



### Install

Git clone this inside the subservice folder of ArozOS, build it and start it up in ArozOS subservice menu.

You can also use it separately as a really basic reverse proxy server that server website based on domain / virtual directories. You can also add another load balancer in front of multiple reverse proxy server nodes if it is necessary in your case. 



### Build

Requirement: Go 1.16 or above

```
cd ~/arozos/subservices
git clone {this_repo}
cd ReverseProxy
./build.sh
```

To start the module, make sure .disabled file does not exists and start arozos system. You should be able to see a new module named "ReverseProxy" pop up in the start menu.

### Usage

#### Proxy Modes

There are two mode in the ReverseProxy Subservice

1. vdir mode (Virtual Dirctories)
2. subd mode (Subdomain Proxying Mode)



Vdir mode proxy web request based on the virtual directories given in the request URL. For example, when configured to redirect /example to example.com, any visits to {your_domain}/example will be proxied to example.com.



Subd mode proxy web request based on sub-domain exists in the request URL. For example, when configured to redirect example.localhost to example.com, any visits that includes example.localhost (e.g. example.localhost/page1) will be proxied to example.com (e.g. example.com/page1)

#### Root Proxy

Root proxy is the main proxy destination where if all proxy root name did not match, the request will be proxied to this request. If you are working with ArozOS system in default configuration, you can set this to localhost:8080 for any unknown request to be handled by the host ArozOS system



