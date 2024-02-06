package libzome

func (a *App) GetUUID() string {
	return a.globalConfig.uuid
}

func (a *App) GetUUIDPretty() string {
	ugly := a.globalConfig.uuid
	return ugly[len(ugly)-8:]
}
