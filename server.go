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

	// init items table
	if _, err = db.Exec(
		fmt.Sprintf("CREATE TABLE IF NOT EXISTS items (%v)", strings.Join([]string{
			"id INT UNSIGNED PRIMARY KEY AUTO_INCREMENT",
			"name CHAR(64)",
			"description VARCHAR(128)",
			"detailed_description VARCHAR(500)",
			"img_path VARCHAR(256)",
			"price INT UNSIGNED",
		}, ",")),
	); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(-1)
	}

	if _, exists := os.LookupEnv("FILL_ITEMS"); exists {
		if rows, err := db.Query("SELECT COUNT(*) FROM items;"); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(-1)
		} else {
			var scanned_rows uint
			rows.Next()
			rows.Scan(&scanned_rows)
			rows.Close()
			if scanned_rows == 0 {
				fill_items(db)
			}
		}
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
	ID, Price                                       uint
	Name, Description, DetailedDescription, ImgPath string
}

func fill_items(db *sql.DB) {
	for _, item := range []Item{
		{
			Name:                "Jeruk",
			Description:         "Buah jeruk segar hasil pertanian.",
			DetailedDescription: "Jeruk (Citrus sinensis) adalah buah segar dengan rasa manis dan asam yang berasal dari pohon jeruk. Buah jeruk sering digunakan untuk diolah menjadi jus segar. Jeruk mengandung banyak vitamin C dan serat yang baik untuk kesehatan tubuh. Buah ini dapat tumbuh subur di daerah beriklim tropis dan subtropis.",
			ImgPath:             "assets/jeruk.jpg",
			Price:               10000,
		},
		{
			Name:                "Pohon Mangga",
			Description:         "Bibit pohon mangga untuk ditanam.",
			DetailedDescription: "Pohon mangga (Mangifera indica) adalah pohon buah yang menghasilkan buah mangga. Buah mangga terkenal dengan rasa manisnya yang lezat. Bibit pohon mangga cocok untuk ditanam di halaman rumah atau kebun. Pohon mangga memerlukan sinar matahari yang cukup dan perawatan yang baik untuk menghasilkan buah yang berkualitas.",
			ImgPath:             "assets/mangga.jpg",
			Price:               12000,
		},
		{
			Name:                "Pupuk Cair Organik",
			Description:         "Pupuk cair organik untuk pertanian.",
			DetailedDescription: "Pupuk cair organik adalah pupuk yang terbuat dari bahan-bahan alami seperti kompos, limbah organik, dan mikroorganisme. Pupuk ini membantu meningkatkan kesuburan tanah dan memberikan nutrisi yang dibutuhkan tanaman. Pupuk cair organik cocok digunakan untuk pertanian organik dan ramah lingkungan.",
			ImgPath:             "assets/pupuk.jpg",
			Price:               15000,
		},
		{
			Name:                "Tebu",
			Description:         "Gula tebu hasil pertanian.",
			DetailedDescription: "Tebu (Saccharum officinarum) adalah tanaman yang menghasilkan tebu, bahan baku untuk gula. Tebu ditanam dalam skala besar untuk menghasilkan gula dalam berbagai bentuk, termasuk gula pasir dan gula cair. Hasil pertanian tebu adalah komoditas penting dalam industri pangan.",
			ImgPath:             "assets/tebu.jpg",
			Price:               11000,
		},
		{
			Name:                "Bibit Kelapa",
			Description:         "Bibit kelapa untuk ditanam.",
			DetailedDescription: "Bibit kelapa adalah tanaman kelapa muda yang siap ditanam. Kelapa adalah salah satu pohon penting dalam ekosistem tropis dan subtropis. Buah kelapa menghasilkan air kelapa segar dan daging kelapa yang dapat digunakan dalam berbagai hidangan.",
			ImgPath:             "assets/kelapa.jpg",
			Price:               13000,
		},
		{
			Name:                "Pupuk NPK",
			Description:         "Pupuk NPK untuk pertanian.",
			DetailedDescription: "Pupuk NPK adalah pupuk komersial yang mengandung campuran nitrogen (N), fosfor (P), dan kalium (K). Nutrisi ini penting untuk pertumbuhan tanaman dan produksi hasil yang baik. Pupuk NPK digunakan secara luas dalam pertanian modern untuk meningkatkan hasil panen.",
			ImgPath:             "assets/npk.jpg",
			Price:               14000,
		},
		{
			Name:                "Tomat",
			Description:         "Tomat segar hasil pertanian.",
			DetailedDescription: "Tomat (Solanum lycopersicum) adalah buah sayuran yang sering digunakan dalam berbagai hidangan. Tomat segar mengandung vitamin C, vitamin A, dan likopen yang baik untuk kesehatan. Buah ini dapat dikonsumsi segar atau digunakan dalam masakan.",
			ImgPath:             "assets/tomat.jpg",
			Price:               12000,
		},
		{
			Name:                "Bibit Jeruk",
			Description:         "Bibit jeruk untuk ditanam.",
			DetailedDescription: "Bibit jeruk adalah tanaman muda dari pohon jeruk yang siap ditanam. Jeruk adalah sumber vitamin C yang baik dan sering digunakan untuk diolah menjadi minuman segar. Tanaman jeruk memerlukan perawatan yang baik untuk menghasilkan buah yang berkualitas.",
			ImgPath:             "assets/bibit-jeruk.jpg",
			Price:               11000,
		},
		{
			Name:                "Pupuk Organik Granular",
			Description:         "Pupuk organik granular untuk pertanian.",
			DetailedDescription: "Pupuk organik granular adalah pupuk yang terbuat dari bahan-bahan organik seperti kompos, pupuk kandang, dan bahan alami lainnya. Pupuk ini membantu meningkatkan kesuburan tanah dan memberikan nutrisi yang dibutuhkan tanaman dengan cara yang ramah lingkungan. Cocok untuk pertanian organik.",
			ImgPath:             "assets/granular.jpg",
			Price:               16000,
		},
		{
			Name:                "Stroberi",
			Description:         "Stroberi segar hasil pertanian.",
			DetailedDescription: "Stroberi (Fragaria × ananassa) adalah buah segar dengan rasa manis yang populer. Stroberi mengandung vitamin C, serat, dan antioksidan yang baik untuk kesehatan. Buah ini sering digunakan dalam hidangan penutup seperti tart dan es krim.",
			ImgPath:             "assets/stroberi.jpg",
			Price:               13000,
		},
	} {
		if _, err := db.Exec(
			"INSERT INTO items(name, description, detailed_description, img_path, price) VALUES(?, ?, ?, ?, ?)",
			item.Name,
			item.Description,
			item.DetailedDescription,
			item.ImgPath,
			item.Price,
		); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return
		}
	}
}
