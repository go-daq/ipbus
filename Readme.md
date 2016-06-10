goIPbus
=======

IPbus2.0 client library implemented in go.

The IPbus protocol allows communication with an FPGA via TCP or UDP.
The client library allows software to read and write values to IPbus registers in the FPGA target.

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

**I am currently working towards getting a stable API with basic functionality tested, which will be released publicly as v1.0.**

