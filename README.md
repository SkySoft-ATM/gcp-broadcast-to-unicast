# gcp-broadcast-to-unicast
Converts the udp broadcast publication to unicast on GCP VMs.

The application looks for the flag ```broadcast_ports``` on the GCP VM instance, this flag should contain a list of port separated by ```_``` (underscore).


If the flag is not present the application will exit.


If the flag is present, the application will start listening on these ports and forward the UDP traffic as unicast (on the same port) to every IP address of all VMs on the same project and zone. 


You can configure the maximum UDP datagram size with the flag ```broadcast_max_datagram_size``` (it defaults to 8192)