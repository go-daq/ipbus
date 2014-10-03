Online SM1 DAQ software
=======================

_The online DAQ software sends and received IPbus packets to/from the GLIBs and stores the raw data to disk._

Raw data format
---------------

The raw data format consists of a run header, a number of IPbus transactions and then a run footer.
The raw format is segmented in units of 32 bit words.

The run header contains the following information:

* header size [words] - 32 bit
* online software commit - sha1 hash 160 bit
* run start time - 64 bit unix time in nanoseconds
* target run stop time - 64 bit unix time in nanoseconds
* Trigger thresholds + settings - ???
* Zero length encoding thresholds + settings - ???


The IPbus transactions are paired (request and reply) and contain the following information:

* remote IP (32 bit)
* remote port (16 bit), entry length (16 bit)
* time request sent - 64 bit integer, unix time in nanoseconds
* time reply received - 64 bit integer, unix time in nanoseconds
* IPbus request packet - n words
* IPbus reply packet - n words

Information from the slow control is included as if it were implemented using the IPbus protocol (requests to read and write registers on slow control devices).

The run footer contains the following information:

* 0x00000000 - to signify there is not another IPbus transaction
* run stop time - 64 bit unix time in nanoseconds
* Reason for end of run? (reached end of expected time, manually stopped, error, etc?)
* ???

If a run does not stop cleanly then the footer may not exist and the final IPbus packet written may not be complete.
