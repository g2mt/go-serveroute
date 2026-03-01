package althost

import "net/http"

type AltHost struct {
	SSH *SSHTunnel `yaml:"ssh"`
}

type Tunnel interface {
	Open() error
	Close()
	Forward(w http.ResponseWriter, r *http.Request)
}
