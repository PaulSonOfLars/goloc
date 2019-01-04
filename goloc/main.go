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
			if l.DefaultLang == language.Und {
				logrus.Fatalf("invalid language selected: '%s' does not match any known language codes")
			}
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

	rootCmd.AddCommand(&cobra.Command{
		Use:   "extract",
		Short: "extract all strings",
		Run: func(cmd *cobra.Command, args []string) {
			if err := l.Handle(args, l.Fix); err != nil {
				logrus.Fatal(err)
			}
		},
	})

	createLang := ""
	createCmd := &cobra.Command{
		Use:   "create",
		Short: "create new language from default",
		Run: func(cmd *cobra.Command, args []string) {
			if createLang == "" {
				logrus.Error("No language to create specified")
				return
			}
			lang := language.Make(createLang)
			if lang == language.Und {
				logrus.Fatalf("invalid language selected: '%s' does not match any known language codes")
			}

			l.Create(args, lang)
		},
	}
	createCmd.Flags().StringVarP(&createLang, "create", "c", "", "select which language to create")
	rootCmd.AddCommand(createCmd)

	checkLang := "all"
	checkCmd := &cobra.Command{
		Use:"check",
		Short: "Check integrity of language files",
		Run: func(cmd *cobra.Command, args []string) {
			var err error
			if checkLang == "all" {
				err = l.CheckAll()
				// load all, iterate over language code
				// check all
			} else {
				lang := language.Make(checkLang)
				if lang == language.Und {
					logrus.Fatalf("invalid language selected: '%s' does not match any known language codes")
				}
				// load default, and load lang, check
				err = l.Check(lang)
			}
			if err != nil {
				logrus.Fatal(err)
			}

		},
	}
	checkCmd.Flags().StringVarP(&checkLang, "check", "c", "all", "select which language to check")
	rootCmd.AddCommand(checkCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
