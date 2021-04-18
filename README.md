# go-kuberoll
## A trivial example service written in golang. 
This service wraps code entirely based on https://github.com/justinian/dice.  The terse way the args are used worked perfectly for this exercise, although I'm only exercising a small bit of its functionality. I forked the repo and have this service pointing at that instead of the original because I've added a random pause during the Roll function to simulate a performance problem.

## Installation
The service makes use of an environment variable to get at the Jaeger Collector. Note the name or IP of that before installing this service.
To install into the appdev namespace
```
$ cd k8s/helm/go-kuberoll
$ kubectl create namespace appdev
$ helm upgrade --install -n appdev \ 
  --set jaegerAgentHost=10.100.105.117 \
  --set service.type=LoadBalancer go-kuberoll .
```

## Features
go-kuberoll exposes endpoints for health, readiness, metrics, and relations (or code path tracing/heat maps) as well as the service's intended functionality.

**/health** returns a 200 status and "OK: 200"  
**/readiness** returns a 200 status after 15 seconds of the container starting. Until then, it generates a 503  
**/metrics** returns prometheus-style metrics by way of https://github.com/labbsr0x/mux-monitor  
**/relations** returns an array of structs meant to demonstrate how a function would provide a way for a mapping service
to interrogate this service to determine its dependencies   
**/** takes a parameter called "**roll**" that is formatted: 

```
xdy[[k|d][h|l]z][+/-c]
     where:
       x = The number of rolls
       y = how many sides the die has
       k|d = keep or drop
       h|l = the highest or lowest
       z = number of rolls to keep or drop
       +/-c = add or subtract a number (c)
``` 
As an example, http://localhost:8080/?roll=4d8 , will sum up four rolls of an 8-sided die.

## Output
Sending a roll parameter to "/", will trigger a performRoll returning output that restates the arg and the result.
```
Received request: 2d4
Result: 5
```

## Telemetry
go-kuberoll makes use of Uber's zap logging package, https://github.com/uber-go/zap. It is fast and has a logger that enforces structured logging. All logs go to standard out. As part of logging, logging for that call includes a roll audit that shows what went into making that result.
It uses the OpenTelemetry, https://opentelemetry.io/,  to send traces to jaeger.


## Special Cases
Specific roll requests are considered special in that they would be used for live system testing, or to reveal what resources or services this service depends upon. These are used for that purpose.  
roll=5d1  
roll=7d1  
roll=9d1  
roll=11d1  

TODO: those currently short circuit the call to the Roll and just return the expected value (5,7,9, or 11 respectively).
      That's okay for ones that might be for basic testing.  Any that would be used for dependencies would return Json 
      listing the dependencies based on an array of a standard struct.

## Relations
Calling the relations endpoint would be done periodically by a service whose responsibility would be to collate resource and service general use data.  That data would be used to answer questions regarding the paths through the system, what are the most popular ones, and which are critical path (which ones make us money!)

## Opportunities 
The logging, tracing, and metrics initialization for the service should be isolated off and generalized in a way that it makes including or switching different options trivial.  In most envs we won't want the expense or expanse of P1 supporting services.   That will also allow for development to be more thorough in appdev phases.

