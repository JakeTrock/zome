package libzome

func (a *App) GetUUID() string {
	return a.globalConfig.uuid
}
