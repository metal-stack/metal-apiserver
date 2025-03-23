package metal

type DNSServers []DNSServer

type DNSServer struct {
	IP string `rethinkdb:"ip" description:"ip address of this dns server"`
}

type NTPServers []NTPServer

type NTPServer struct {
	Address string `address:"address" description:"ip address or dns hostname of this ntp server"`
}
