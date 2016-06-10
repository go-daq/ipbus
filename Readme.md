goIPbus
=======

IPbus2.0 client library implemented in go.

The IPbus protocol allows communication with an FPGA via TCP or UDP.
The IPbus protocol was originally designed for data acquisition systems for the LHC experiments at CERN.
Documentation on the IPbus library can be found at https://svnweb.cern.ch/trac/cactus/wiki/uhalQuickTutorial.
The client library allows software to read and write values to IPbus registers in the FPGA target.

The Go implementation was first developed as part of the read-out system of the 288 kg prototype SoLid anti-neutrino detector (http://arxiv.org/abs/1510.07835).
The code was then split into a package for use in the full scale SoLid detector.

Installation
------------

Once you have set up your Go work area you can instal the goipbus package with the following command:

````
go get bitbucket.org/NickRyder/goipbus/ipbus
````

and then use it in your code, such as:
````
#!go
package main

import (
    "bitbucket.org/NickRyder/goipbus/ipbus"
    "net"
)

func main() 
    conn, err := net.Dial("udp4", "localhost:50001")
    // Handle error...
    fn := "hardwaredescription.xml"
    target := ipbus.New("DAQ", fn, conn)
    fifo := target.Regs["FIFO"]
    repchan := target.Read(fifo, 1024)
    // target.Dispatch() // This would block until all packets are received
    go target.Dispatch() // This lets you handle each packet as it arrives
    for rep := range repchan {
        if rep.Err != nil {
            //handle error
        }
        process(rep.Data)
    }
}
````
Dependencies
------------

The goipbus package does not depend on any packages outside the go standard library.
The package was developed and tested using go version go1.6.2 linux/amd64.
It is expected to work with any go1.x.

Fully testing the package requires the C++ IPbus implemenation to be installed (see https://svnweb.cern.ch/trac/cactus/wiki/uhalQuickTutorial#HowtoInstalltheIPbusSuite) because tests are run which communicate with the dummy hardware included in that package.
*Package users do not need to install the C++ library.*
You can skip all tests that require the dummy hardware with the command:

````
go test -nodummyhardware
```` 

License
-------

This software is released under a 3-clause BSD license, see [[License.md]] or call `ipbus.License()`.

Versions
--------

The current version of the goipbus package is 0.1.
The version can be accessed as `ipbus.PackageVersion`.

**I am currently working towards getting a stable API with basic functionality tested, which will be released publicly as v1.0.**

Logo
----

The goipbus logo uses the `gophercolor` image (see https://golang.org/doc/gopher/) designed by Renee French.
