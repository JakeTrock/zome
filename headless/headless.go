package headless

import (
	"context"
	"flag"
	"fmt"

	libzome "zome/libzome"
)

func main() {
	deviceIdOverride := flag.String("overrideId", "", "overrides default device id")
	poolIdOverride := flag.String("overridePool", "", "overrides default pool id")
	configPathOverride := flag.String("configPath", "", "overrides default config path")
	help := flag.Bool("h", false, "Display Help")
	flag.Parse()
	if *help {
		fmt.Println("zome under construction")
		fmt.Println()
		flag.PrintDefaults()
		return
	}

	zomeBackend := libzome.NewApp(map[string]string{
		"deviceId":   *deviceIdOverride,
		"poolId":     *poolIdOverride,
		"configPath": *configPathOverride,
	})

	ctx := context.Background()
	zomeBackend.Startup(ctx)

}

//Go sse: https://github.com/gofiber/recipes/blob/master/sse/main.go
