package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
)

func main() {
	// init dotenv
	godotenv.Load()

	// check environment variables
	env := map[string]string{
		"LISTEN_PORT": "",
		"DB_USERNAME": "",
		"DB_PASSWORD": "",
		"DB_NAME":     "",
	}
	for k := range env {
		value, exists := os.LookupEnv(k)
		if !exists {
			fmt.Fprintln(os.Stderr, fmt.Errorf("Ensure \"%v\" is defined", k))
			os.Exit(-1)
		}
		env[k] = value
	}

	// init db
	db, err := sql.Open(
		"mysql",
		fmt.Sprintf("%v:%v@/", env["DB_USERNAME"], env["DB_PASSWORD"]),
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(-1)
	}
	if err := db.Ping(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(-1)
	}
	if _, err := db.Exec(fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %v", env["DB_NAME"])); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(-1)
	}
	if _, err := db.Exec(fmt.Sprintf("USE %v", env["DB_NAME"])); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(-1)
	}
	defer db.Close()

	// init users table
	if _, err = db.Exec(
		fmt.Sprintf("CREATE TABLE IF NOT EXISTS users (%v)", strings.Join([]string{
			"id INT UNSIGNED PRIMARY KEY AUTO_INCREMENT",
			"username CHAR(20)",
			"password_hash CHAR(64)",
		}, ",")),
	); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(-1)
	}

	// init items table
	if _, err = db.Exec(
		fmt.Sprintf("CREATE TABLE IF NOT EXISTS items (%v)", strings.Join([]string{
			"id INT UNSIGNED PRIMARY KEY AUTO_INCREMENT",
			"name CHAR(64)",
			"description VARCHAR(128)",
			"detailed_description VARCHAR(256)",
			"img_path VARCHAR(256)",
			"price INT UNSIGNED",
		}, ",")),
	); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(-1)
	}

	// init web server
	s := http.Server{
		Addr: fmt.Sprintf(":%s", env["LISTEN_PORT"]),
		Handler: WsHandler{
			DB: db,
		},
	}
	defer s.Close()

	sError := make(chan error)
	go func() {
		e := s.ListenAndServe()
		sError <- e
		close(sError)
	}()
	fmt.Printf("Listening on port %s...\n", env["LISTEN_PORT"])
	fmt.Fprintln(os.Stderr, <-sError)
	os.Exit(-1)
}

type WsHandler struct {
	DB *sql.DB
}

func (wsh WsHandler) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	switch req.URL.Path {
	case "/create":
		if req.Method == "POST" {
			if err := req.ParseForm(); err != nil {
				fmt.Fprintln(os.Stderr, err)
				res.WriteHeader(500)
				return
			} else {
				// init itemMap
				itemMap := map[string]string{}
				for _, v := range []string{
					"Name",
					"Description",
					"DetailedDescription",
					"ImgPath",
					"Price",
				} {
					if !req.PostForm.Has(v) {
						res.WriteHeader(400)
						return
					} else {
						itemMap[v] = req.PostForm.Get(v)
					}
				}

				// insert item into db
				if _, err := wsh.DB.Exec(
					"INSERT INTO items(name, description, detailed_description, img_path, price) VALUES(?, ?, ?, ?, ?)",
					itemMap["Name"],
					itemMap["Description"],
					itemMap["DetailedDescription"],
					itemMap["ImgPath"],
					itemMap["Price"],
				); err != nil {
					fmt.Fprintln(os.Stderr, err)
					res.WriteHeader(400)
					return
				} else {
					res.WriteHeader(201)
					return
				}
			}
		}
	case "/read":
		if req.Method == "GET" {
			if rows, err := wsh.DB.Query("SELECT * FROM items"); err != nil {
				fmt.Fprintln(os.Stderr, err)
				res.WriteHeader(500)
				return
			} else {
				// init items
				var items []Item
				for rows.Next() {
					item := Item{}
					if err := rows.Scan(
						&item.ID,
						&item.Name,
						&item.Description,
						&item.DetailedDescription,
						&item.ImgPath,
						&item.Price,
					); err != nil {
						fmt.Fprintln(os.Stderr, err)
						res.WriteHeader(500)
						return
					} else {
						items = append(items, item)
					}
				}

				// encode items to json
				if itemsJson, err := json.Marshal(items); err != nil {
					fmt.Fprintln(os.Stderr, err)
					res.WriteHeader(500)
				} else {
					res.Header().Add("Content-Type", "application/json")
					res.Write(itemsJson)
				}
				return
			}
		}
	case "/update":
		if req.Method == "PUT" {
			if err := req.ParseForm(); err != nil {
				fmt.Fprintln(os.Stderr, err)
				res.WriteHeader(500)
				return
			} else {
				// init itemMap
				itemMap := map[string]string{}
				for _, v := range []string{
					"ID",
					"Name",
					"Description",
					"DetailedDescription",
					"ImgPath",
					"Price",
				} {
					if !req.PostForm.Has(v) {
						res.WriteHeader(400)
						return
					} else {
						itemMap[v] = req.PostForm.Get(v)
					}
				}

				// update item
				if _, err := wsh.DB.Exec(
					"UPDATE items SET name=?, description=?, detailed_description=?, img_path=?, price=? WHERE id=?",
					itemMap["Name"],
					itemMap["Description"],
					itemMap["DetailedDescription"],
					itemMap["ImgPath"],
					itemMap["Price"],
					itemMap["ID"],
				); err != nil {
					res.WriteHeader(400)
				} else {
					res.WriteHeader(200)
				}
			}
			return
		}
	case "/delete":
		if req.Method == "DELETE" {
			if err := req.ParseForm(); err != nil {
				fmt.Fprintln(os.Stderr, err)
				res.WriteHeader(500)
				return
			} else {
				if !req.Form.Has("ID") {
					res.WriteHeader(400)
					return
				} else {
					if _, err := wsh.DB.Exec(
						"DELETE FROM items WHERE ID=?",
						req.Form.Get("ID"),
					); err != nil {
						res.WriteHeader(400)
					} else {
						res.WriteHeader(200)
					}
					return
				}
			}
		}
	}
	res.WriteHeader(404)
}

type Item struct {
	ID, Name, Description, DetailedDescription, ImgPath string
	Price                                               uint
}
