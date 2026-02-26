package service

type ServiceType int

const (
	ServiceTypeUnknown ServiceType = iota
	ServiceTypeFiles
	ServiceTypeProxy
	ServiceTypeAPI
)

type Service struct {
	subdomain string `yaml:"subdomain"`
	hidden    bool   `yaml:"hidden"`

	serveFiles string `yaml:"serve_files"`
	forwardsTo string `yaml:"forwards_to"`
	api        bool   `yaml:"api"`

	start       []string `yaml:"start"`
	stop        []string `yaml:"stop"`
	timeout     int      `yaml:"timeout"`
	killTimeout int      `yaml:"kill_timeout"`
}

func (s *Service) GetSubdomain() string {
	return s.subdomain
}

func (s *Service) GetHidden() bool {
	return s.hidden
}

func (s *Service) GetForwardsTo() string {
	return s.forwardsTo
}

func (s *Service) GetServeFiles() string {
	return s.serveFiles
}

func (s *Service) GetTimeout() int {
	return s.timeout
}

func (s *Service) Type() ServiceType {
	if s.serveFiles != "" {
		return ServiceTypeFiles
	}
	if s.forwardsTo != "" {
		return ServiceTypeProxy
	}
	if s.api {
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
		servicesBySubdomain[svc.subdomain] = NamedService{Name: name, Svc: svc}
	}
	return servicesBySubdomain
}
