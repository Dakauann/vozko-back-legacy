package sitepublish

func PrimaryOrigin(list []HostnameRecord, origins Origins) (string, bool) {
	platform := ""
	for _, h := range list {
		if h.Status != HostnameActive {
			continue
		}
		if h.Kind == HostnameCustom {
			return origins.Of(h.Hostname), true
		}
		if platform == "" {
			platform = h.Hostname
		}
	}
	if platform == "" {
		return "", false
	}
	return origins.Of(platform), true
}
