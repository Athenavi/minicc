package skill

// DiscoverResult describes a skill found during discovery.
type DiscoverResult struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Version     string `json:"version,omitempty"`
	Author      string `json:"author,omitempty"`
	Source      string `json:"source,omitempty"` // URL or path
	Installed   bool   `json:"installed"`        // true if already in the store
}

// Discoverer scans local and remote sources for available skills.
type Discoverer struct {
	store     *SkillStore
	installer *Installer
}

func NewDiscoverer(store *SkillStore, installer *Installer) *Discoverer {
	return &Discoverer{
		store:     store,
		installer: installer,
	}
}

// DiscoverLocal scans the skills directory for available .skill.json files
// and returns a list of skills not yet installed.
// For simplicity in this implementation, all local skills are already installed
// via the store. This method returns installed skills metadata for listing.
func (d *Discoverer) DiscoverLocal() ([]DiscoverResult, error) {
	installed := d.store.List()
	results := make([]DiscoverResult, 0, len(installed))
	for _, sk := range installed {
		results = append(results, DiscoverResult{
			Name:        sk.Name,
			Description: sk.Description,
			Version:     sk.Version,
			Author:      sk.Author,
			Source:      sk.Source,
			Installed:   true,
		})
	}
	return results, nil
}

// DiscoverRemote fetches a skill index from a remote URL and returns
// discoverable skills, marking which are already installed.
func (d *Discoverer) DiscoverRemote(indexURL string) ([]DiscoverResult, error) {
	remoteSkills, err := d.installer.DiscoverRemoteSkills(nil, indexURL)
	if err != nil {
		return nil, err
	}

	results := make([]DiscoverResult, 0, len(remoteSkills))
	for _, sk := range remoteSkills {
		installed := d.store.Get(sk.Name) != nil
		results = append(results, DiscoverResult{
			Name:        sk.Name,
			Description: sk.Description,
			Version:     sk.Version,
			Author:      sk.Author,
			Source:      sk.Source,
			Installed:   installed,
		})
	}
	return results, nil
}
