package nexhost

// Title is one game hosted by the shared NEX binary. The binary picks the title named by
// NEX_TITLE, applies its Defaults, lets the environment override them, builds the Host and
// calls Register before Run.
//
// ponytail: one title per process. Several titles in one process would need per-title env
// prefixes, a recover wrapper around nex-go's handlers (a panic in one title must not take
// the others down) and a fix for nex-protocols-common-go's package globals (BuildName, the
// gRPC account client). Until then the image is shared and the container is per title.
type Title struct {
	Name     string            // the NEX_TITLE value that selects it; also the default Config.Title
	Defaults func(*Config)     // per-title values the environment may still override
	Register func(*Host) error // registers the title's protocols on host.Secure
}

// Config is the title's configuration as the binary builds it: Defaults, then the title's
// own values, then the environment.
func (t Title) Config() Config {
	c := Defaults()
	if t.Name != "" {
		c.Title = t.Name
	}
	if t.Defaults != nil {
		t.Defaults(&c)
	}
	return FromEnvWith(c)
}
