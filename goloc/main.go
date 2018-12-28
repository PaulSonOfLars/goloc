package main

import (
	"fmt"
	"github.com/PaulSonOfLars/goloc"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"go/token"
	"golang.org/x/text/language"
	"os"
)

func main() {
	l := &goloc.Locer{
		DefaultLang: language.BritishEnglish,
		Checked:     make(map[string]struct{}),
		Fset: token.NewFileSet(),
	}
	var lang string
	verbose := false

	rootCmd := cobra.Command{
		Use:   "goloc",
		Short: "Extract strings for i18n of your go tools",
		Long:  "Simple i18n tool to allow for extracting all your i18n strings into manageable files, and load them back after.",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if verbose {
				logrus.SetLevel(logrus.DebugLevel)
			} else {
				logrus.SetLevel(logrus.InfoLevel)
			}
			l.DefaultLang = language.Make(lang)
		},
	}
	rootCmd.PersistentFlags().StringSliceVar(&l.Funcs, "funcs", nil, "all funcs to extraxt")
	rootCmd.PersistentFlags().StringSliceVar(&l.Fmtfuncs, "fmtfuncs", nil, "all format funcs to extract")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "add extra verbosity")
	rootCmd.PersistentFlags().BoolVarP(&l.Apply, "apply", "a", false, "save to file")
	rootCmd.PersistentFlags().StringVarP(&lang, "lang", "l", language.BritishEnglish.String(), "")

	rootCmd.AddCommand(&cobra.Command{
		Use:   "inspect",
		Short: "Run an analyse all appropriate strings in specified files",
		Run: func(cmd *cobra.Command, args []string) {
			if err := l.Handle(args, l.Inspect); err != nil {
				logrus.Fatal(err)
			}
		},
	})

	//rootCmd.AddCommand(&cobra.Command{
	//	Use:   "extract",
	//	Short: "extract all strings",
	//	Run: func(cmd *cobra.Command, args []string) {
	//		l.handle(args, l.inspect)
	//		l.extract()
	//	},
	//})

	rootCmd.AddCommand(&cobra.Command{
		Use:   "extract",
		Short: "extract all strings",
		Run: func(cmd *cobra.Command, args []string) {
			l.Handle(args, l.Fix)
		},
	})

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
