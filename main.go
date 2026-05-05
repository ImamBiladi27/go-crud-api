// package main menandakan ini adalah entry point program (file yang dapat dieksekusi)
package main

import (
	// interface generik untuk operasi database SQL (seperti Query, Exec, QueryRow)
	"database/sql"
	// package untuk pencatatan log (digunakan untuk log.Fatal jika terjadi error fatal)
	"log"
	// package HTTP untuk status code standar seperti StatusOK, StatusNotFound, dll
	"net/http"
	// package untuk mengakses environment variable sistem operasi (digunakan untuk membaca DATABASE_URL)
	"os"

	// framework HTTP Gin — menyediakan router, handler context, JSON binding, dan middleware
	"github.com/gin-gonic/gin"
	// driver PostgreSQL; diimpor dengan blank identifier _ hanya untuk efek sampingnya (mendaftarkan driver "postgres" ke database/sql)
	_ "github.com/lib/pq"
)

// User merepresentasikan entitas pengguna dalam database
type User struct {
	// ID unik pengguna (primary key, auto-increment oleh PostgreSQL)
	ID int `json:"id"`
	// Name adalah nama pengguna
	Name string `json:"name"`
	// Email adalah alamat email pengguna
	Email string `json:"email"`
}

// main adalah fungsi entry point program — dijalankan pertama kali saat aplikasi dimulai
func main() {
	// buka koneksi ke PostgreSQL menggunakan driver "postgres" dan URL dari environment variable DATABASE_URL
	db, err := sql.Open("postgres", os.Getenv("DATABASE_URL"))
	// jika koneksi gagal dibuka, hentikan program dengan mencatat error dan keluar dengan status non-zero
	if err != nil {
		log.Fatal(err)
	}
	// pastikan koneksi database ditutup saat fungsi main selesai (cleanup resource)
	defer db.Close()

	// jalankan query untuk membuat tabel users jika belum ada (CREATE TABLE IF NOT EXISTS)
	// tabel memiliki kolom: id (auto-increment integer), name (teks), email (teks)
	_, err = db.Exec("CREATE TABLE IF NOT EXISTS users (id SERIAL PRIMARY KEY, name TEXT, email TEXT)")
	// jika pembuatan tabel gagal, hentikan program
	if err != nil {
		log.Fatal(err)
	}

	// buat instance router Gin dengan konfigurasi default (sudah termasuk middleware Logger dan Recovery)
	router := gin.Default()

	// daftarkan route HTTP — setiap route memetakan metode + path ke handler function
	// GET /users → ambil semua pengguna
	router.GET("/users", getUsers(db))
	// GET /users/:id → ambil satu pengguna berdasarkan ID dari path parameter
	router.GET("/users/:id", getUser(db))
	// POST /users → buat pengguna baru dari JSON body request
	router.POST("/users", createUser(db))
	// PUT /users/:id → perbarui data pengguna berdasarkan ID
	router.PUT("/users/:id", updateUser(db))
	// DELETE /users/:id → hapus pengguna berdasarkan ID
	router.DELETE("/users/:id", deleteUser(db))

	// jalankan HTTP server di port 8000; router.Run akan memblokir (blocking) dan mengembalikan error jika server gagal
	// log.Fatal mencatat error dan menghentikan program jika server gagal berjalan
	log.Fatal(router.Run(":8000"))
}

// getUsers mengembalikan gin.HandlerFunc untuk route GET /users
// menerima pointer *sql.DB agar handler bisa mengakses database
func getUsers(db *sql.DB) gin.HandlerFunc {
	// mengembalikan closure (fungsi anonim) yang menangkap variabel db
	return func(c *gin.Context) {
		// jalankan query SELECT * untuk mengambil semua baris dari tabel users
		rows, err := db.Query("SELECT * FROM users")
		// jika query gagal, kembalikan response HTTP 500 Internal Server Error dalam format JSON
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		// pastikan hasil query (rows) ditutup setelah fungsi selesai untuk mencegah kebocoran resource
		defer rows.Close()

		// inisialisasi slice kosong untuk menampung semua data pengguna
		users := []User{}
		// iterasi setiap baris hasil query; rows.Next() menggerakkan kursor ke baris berikutnya
		for rows.Next() {
			// deklarasikan variabel u bertipe User untuk menampung hasil scan satu baris
			var u User
			// scan nilai kolom dari baris saat ini ke dalam field struct User
			// menggunakan pointer ke field agar nilai dapat ditulis ke variabel
			if err := rows.Scan(&u.ID, &u.Name, &u.Email); err != nil {
				// jika scan gagal, kembalikan HTTP 500
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			// tambahkan user yang berhasil di-scan ke dalam slice users
			users = append(users, u)
		}
		// setelah iterasi selesai, cek apakah ada error selama proses iterasi berlangsung
		if err := rows.Err(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		// kirim response HTTP 200 OK dengan body JSON berisi slice users
		c.JSON(http.StatusOK, users)
	}
}

// getUser mengembalikan gin.HandlerFunc untuk route GET /users/:id
// path parameter :id akan diambil dengan c.Param("id")
func getUser(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// ambil nilai path parameter :id dari URL (misal /users/3 → id = "3")
		id := c.Param("id")

		// deklarasikan variabel u untuk hasil query
		var u User
		// QueryRow menjalankan SELECT ... WHERE id = $1 dan mengembalikan maksimal satu baris
		// $1 adalah parameterized query placeholder yang mencegah SQL injection
		err := db.QueryRow("SELECT * FROM users WHERE id = $1", id).Scan(&u.ID, &u.Name, &u.Email)
		// jika tidak ada baris yang cocok, QueryRow mengembalikan sql.ErrNoRows yang ditangkap di sini
		if err != nil {
			// kembalikan HTTP 404 Not Found karena pengguna tidak ditemukan
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}

		// kembalikan HTTP 200 OK dengan body JSON berisi data pengguna
		c.JSON(http.StatusOK, u)
	}
}

// createUser mengembalikan gin.HandlerFunc untuk route POST /users
// menerima JSON body dan memasukkan data ke database
func createUser(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// deklarasikan variabel u untuk menampung hasil binding dari body JSON request
		var u User
		// ShouldBindJSON membaca body request, mem-parse JSON, dan mengisi field struct u
		// jika body tidak valid JSON atau struktur tidak sesuai, akan mengembalikan error
		if err := c.ShouldBindJSON(&u); err != nil {
			// kembalikan HTTP 400 Bad Request jika JSON tidak valid
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// jalankan INSERT dan gunakan RETURNING id untuk mendapatkan ID yang di-generate oleh PostgreSQL
		// $1 dan $2 diisi oleh u.Name dan u.Email
		err := db.QueryRow("INSERT INTO users (name, email) VALUES ($1, $2) RETURNING id", u.Name, u.Email).Scan(&u.ID)
		// jika insert gagal (misal constraint violation), kembalikan HTTP 500
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		// kembalikan HTTP 200 OK dengan body JSON berisi user yang baru dibuat (termasuk ID barunya)
		c.JSON(http.StatusOK, u)
	}
}

// updateUser mengembalikan gin.HandlerFunc untuk route PUT /users/:id
// menerima JSON body dan memperbarui data pengguna berdasarkan ID
func updateUser(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// ambil nilai path parameter :id dari URL
		id := c.Param("id")

		// deklarasikan variabel u untuk menampung data dari body request
		var u User
		// binding JSON body request ke struct u; kembalikan 400 jika gagal
		if err := c.ShouldBindJSON(&u); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// jalankan UPDATE untuk mengubah name dan email pengguna dengan ID yang sesuai
		// menggunakan parameterized query ($1, $2, $3) untuk mencegah SQL injection
		result, err := db.Exec("UPDATE users SET name = $1, email = $2 WHERE id = $3", u.Name, u.Email, id)
		// jika query gagal, kembalikan HTTP 500
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		// periksa jumlah baris yang terpengaruh oleh operasi UPDATE
		rowsAffected, _ := result.RowsAffected()
		// jika tidak ada baris yang berubah, artinya pengguna dengan ID tersebut tidak ditemukan
		if rowsAffected == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}

		// kembalikan HTTP 200 OK dengan body JSON berisi data yang baru saja di-update
		c.JSON(http.StatusOK, u)
	}
}

// deleteUser mengembalikan gin.HandlerFunc untuk route DELETE /users/:id
// menghapus pengguna dari database berdasarkan ID
func deleteUser(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// ambil nilai path parameter :id dari URL
		id := c.Param("id")

		// deklarasikan variabel u untuk mengecek apakah user dengan ID tersebut ada
		var u User
		// cek keberadaan user terlebih dahulu dengan SELECT
		err := db.QueryRow("SELECT * FROM users WHERE id = $1", id).Scan(&u.ID, &u.Name, &u.Email)
		// jika user tidak ditemukan, kembalikan HTTP 404
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}

		// hapus user dengan ID yang diberikan menggunakan parameterized query
		_, err = db.Exec("DELETE FROM users WHERE id = $1", id)
		// jika penghapusan gagal, kembalikan HTTP 500
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		// kembalikan HTTP 200 OK dengan pesan sukses dalam format JSON
		c.JSON(http.StatusOK, gin.H{"message": "User deleted"})
	}
}
