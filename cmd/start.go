package cmd

import (
	"github.com/bestruirui/octopus/internal/conf"
	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/op"
	"github.com/bestruirui/octopus/internal/server"
	"github.com/bestruirui/octopus/internal/sidecar"
	"github.com/bestruirui/octopus/internal/task"
	"github.com/bestruirui/octopus/internal/utils/shutdown"
	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"
)

var cfgFile string
var withO2a bool

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start " + conf.APP_NAME,
	PreRun: func(cmd *cobra.Command, args []string) {
		conf.PrintBanner()
		conf.Load(cfgFile)
		if level, err := log.ParseLevel(conf.AppConfig.Log.Level); err == nil {
			log.SetLevel(level)
		}
	},
	Run: func(cmd *cobra.Command, args []string) {
		shutdown.Init(log.Default())
		if err := db.InitDB(conf.AppConfig.Database.Type, conf.AppConfig.Database.Path, conf.IsDebug()); err != nil {
			log.Errorf("database init error: %v", err)
			return
		}
		shutdown.Register(db.Close)

		if err := op.InitCache(); err != nil {
			log.Errorf("cache init error: %v", err)
			return
		}
		shutdown.Register(op.SaveCache)

		if err := op.UserInit(); err != nil {
			log.Errorf("user init error: %v", err)
			return
		}

		if err := server.Start(); err != nil {
			log.Errorf("server start error: %v", err)
			return
		}
		shutdown.Register(server.Close)

		// 侧车默认关闭：仅 --o2a 拉起本轮；无参时绝不自动拉起（配置 enabled 仅面板展示，不参与启动决策）。
		if withO2a {
			sc := sidecar.New(conf.AppConfig.Sidecar.Binary, conf.AppConfig.Sidecar.APIAddr, conf.AppConfig.Sidecar.Config)
			if err := sc.Start(conf.AppConfig.Sidecar.AdminAddr); err != nil {
				log.Errorf("sidecar start error: %v", err)
				return
			}
			shutdown.Register(sc.Close)
			sidecar.SetGlobal(sc)
		}

		task.Init()
		go task.RUN()
		shutdown.Listen()
	},
}

func init() {
	startCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./data/config.json)")
	startCmd.Flags().BoolVar(&withO2a, "o2a", false, "enable Zen sidecar (opencode2api) for this run")
	rootCmd.AddCommand(startCmd)
}
