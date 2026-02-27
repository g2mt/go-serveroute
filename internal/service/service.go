package service

type ServiceType int

const (
	ServiceTypeUnknown ServiceType = iota
	ServiceTypeFiles
	ServiceTypeProxy
	ServiceTypeAPI
)

type Service struct {
	Subdomain string `yaml:"subdomain"`
	Hidden    bool   `yaml:"hidden"`

	ServeFiles string `yaml:"serve_files"`
	ForwardsTo string `yaml:"forwards_to"`
	API        bool   `yaml:"api"`

	Autostart   bool     `yaml:"autostart"`
	Start       []string `yaml:"start"`
	Stop        []string `yaml:"stop"`
	Timeout     int      `yaml:"timeout"`
	KillTimeout int      `yaml:"kill_timeout"`
}

func (s *Service) Type() ServiceType {
	if s.ServeFiles != "" {
		return ServiceTypeFiles
	}
	if s.ForwardsTo != "" {
		return ServiceTypeProxy
	}
	if s.API {
		return ServiceTypeAPI
	}

	return ServiceTypeUnknown
}

type NamedService struct {
	Name string
	Svc  *Service
}

func MakeServicesBySubdomain(services map[string]*Service) map[string]NamedService {
	servicesBySubdomain := make(map[string]NamedService)
	for name, svc := range services {
		servicesBySubdomain[svc.Subdomain] = NamedService{Name: name, Svc: svc}
	}
	return servicesBySubdomain
}
