package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/spf13/cobra"
)

func addDBCommands(root *cobra.Command) {
	var force bool
	drop := &cobra.Command{
		Use:     "db:drop <name>",
		Short:   "Delete a MySQL database",
		Long:    "Delete a MySQL database and all its data. Asks for confirmation unless --force.\n\nExamples:\n  apnoro db:drop shop\n  apnoro db:drop shop --force",
		GroupID: "db",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if !force {
				if fi, err := os.Stdin.Stat(); err != nil || fi.Mode()&os.ModeCharDevice == 0 {
					return errors.New("refusing to drop a database without --force when not interactive")
				}
				if !confirm(fmt.Sprintf("Drop database %q and all of its data?", name)) {
					return errors.New("aborted")
				}
			}
			if err := backend().DropDatabase(name); err != nil {
				return err
			}
			ok("Database %s dropped", name)
			return nil
		},
	}
	drop.Flags().BoolVarP(&force, "force", "f", false, "do not ask for confirmation")

	root.AddCommand(
		&cobra.Command{
			Use:     "db:list",
			Short:   "List MySQL databases",
			Long:    "List the MySQL databases (system schemas excluded). MySQL must be running.\n\nExample:\n  apnoro db:list",
			GroupID: "db",
			Args:    cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				dbs, err := backend().ListDatabases()
				if err != nil {
					return err
				}
				if len(dbs) == 0 {
					fmt.Println("No databases yet. Create one with `apnoro db:create <name>`.")
					return nil
				}
				w := newTable()
				row(w, "DATABASE", "TABLES", "SIZE")
				for _, d := range dbs {
					size := humanBytes(d.SizeBytes)
					if size == "" {
						size = "0 B"
					}
					row(w, d.Name, strconv.Itoa(d.Tables), size)
				}
				return w.Flush()
			},
		},
		&cobra.Command{
			Use:     "db:create <name>",
			Short:   "Create a MySQL database",
			Long:    "Create a MySQL database (utf8mb4). MySQL must be running.\n\nExample:\n  apnoro db:create shop",
			GroupID: "db",
			Args:    cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				if err := backend().CreateDatabase(args[0]); err != nil {
					return err
				}
				ok("Database %s created", args[0])
				return nil
			},
		},
		drop,
		&cobra.Command{
			Use:     "db:import <database> <file.sql>",
			Short:   "Import an SQL file into a database",
			Long:    "Import an SQL dump into an existing database.\n\nExample:\n  apnoro db:import shop backup.sql",
			GroupID: "db",
			Args:    cobra.ExactArgs(2),
			RunE: func(cmd *cobra.Command, args []string) error {
				file, err := filepath.Abs(args[1])
				if err != nil {
					return err
				}
				if _, err := os.Stat(file); err != nil {
					return fmt.Errorf("cannot read %s: %w", file, err)
				}
				fmt.Printf("Importing %s into %s...\n", file, args[0])
				if err := backend().ImportSQL(args[0], file); err != nil {
					return err
				}
				ok("Imported %s into %s", filepath.Base(file), args[0])
				return nil
			},
		},
		&cobra.Command{
			Use:     "db:export <database> <file.sql>",
			Short:   "Export a database to an SQL file",
			Long:    "Dump a database to an SQL file (mysqldump).\n\nExample:\n  apnoro db:export shop shop.sql",
			GroupID: "db",
			Args:    cobra.ExactArgs(2),
			RunE: func(cmd *cobra.Command, args []string) error {
				file, err := filepath.Abs(args[1])
				if err != nil {
					return err
				}
				if err := backend().ExportDatabase(args[0], file); err != nil {
					return err
				}
				ok("Exported %s to %s", args[0], file)
				return nil
			},
		},
	)
}
