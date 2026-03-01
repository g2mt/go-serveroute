package althost

import "net/http"

type AltHost struct {
	SSH *SSHTunnel `yaml:"ssh"`
}

func (ah *AltHost) GetTunnel() Tunnel {
	return ah.SSH
}

type Tunnel interface {
	Open() error
	Close()
	Forward(w http.ResponseWriter, r *http.Request)
}
