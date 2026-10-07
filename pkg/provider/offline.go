package provider

// OfflineIssue describes one configured provider that is not offline.
type OfflineIssue struct {
	Role        string `json:"role"`
	ProviderKey string `json:"provider_key"`
	Tier        Tier   `json:"tier"`
	Reason      string `json:"reason"`
}

// OfflineReport lists configured providers that are not offline.
type OfflineReport struct {
	Offline bool           `json:"offline"`
	Issues  []OfflineIssue `json:"issues"`
}

// InspectOffline checks a single provider key against the registry. If the
// provider is not registered or does not declare FeatureOffline, an OfflineIssue
// is returned with ok=false.
func InspectOffline(role string, key Key) (OfflineIssue, bool) {
	lookupKey := string(key.Parent())
	if lookupKey == "" {
		lookupKey = string(key)
	}
	if Key(lookupKey) == KeyEmbeddingBuiltin {
		return OfflineIssue{}, true
	}
	reg, ok := Lookup(lookupKey)
	if !ok {
		return OfflineIssue{
			Role:        role,
			ProviderKey: string(key),
			Reason:      "unknown provider",
		}, false
	}

	for _, f := range reg.Descriptor.Features {
		if f == FeatureOffline {
			return OfflineIssue{}, true
		}
	}

	reason := "reaches external network or cloud services"
	if reg.Descriptor.Tier == TierLocalServer {
		reason = "local server, not offline"
	} else if reg.Descriptor.Caveat != "" {
		reason = reg.Descriptor.Caveat
	}

	return OfflineIssue{
		Role:        role,
		ProviderKey: string(key),
		Tier:        reg.Descriptor.Tier,
		Reason:      reason,
	}, false
}
