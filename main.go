package main

import (
	"flag"
	"fmt"
	"net/http"
	"simplegallery/controllers"
	"simplegallery/email"
	"simplegallery/middleware"
	"simplegallery/models"
	"simplegallery/rand"

	"golang.org/x/oauth2"

	"github.com/gorilla/csrf"
	"github.com/gorilla/mux"
)

func main() {
	prod := flag.Bool("prod", false, "Set true in production, to force reading of .config file.")
	flag.Parse()

	cfg := LoadConfig(*prod)
	dbCfg := cfg.Database

	serverPort := cfg.GetPort()

	services, err := models.NewServices(
		models.WithGorm(dbCfg.ConnectionInfo()),
		models.WithLogMode(!cfg.IsProd()),
		models.WithUser(cfg.Pepper, cfg.HMACKey),
		models.WithGallery(),
		models.WithImage(),
		models.WithOAuth(),
	)

	must(err)

	defer services.Close()
	//services.DestructiveReset()
	services.AutoMigrate()

	mgCfg := cfg.Mailgun
	emailer := email.NewClient(
		email.WithSender("SimpleGallery Support", "support@sandboxedbdc3b36f894f5b8edeb7c47e599964.mailgun.org"),
		email.WithMailgun(mgCfg.Domain, mgCfg.APIKey, mgCfg.PublicAPIKey),
	)

	r := mux.NewRouter()
	staticC := controllers.NewStatic()
	usersC := controllers.NewUsers(services.User, emailer)
	galleriesC := controllers.NewGalleries(services.Gallery, services.Image, r)

	configs := make(map[string]*oauth2.Config)
	configs[models.OAuthDropbox] = &oauth2.Config{
		ClientID:     cfg.Dropbox.ID,
		ClientSecret: cfg.Dropbox.Secret,
		Endpoint: oauth2.Endpoint{
			AuthURL:  cfg.Dropbox.AuthURL,
			TokenURL: cfg.Dropbox.TokenURL,
		},
		RedirectURL: "http://localhost:3333/oauth/dropbox/callback",
	}

	oauthsC := controllers.NewOAuths(services.OAuth, configs)

	b, err := rand.Bytes(32)
	must(err)
	csrfMw := csrf.Protect(b, csrf.Secure(cfg.IsProd()))
	userMw := middleware.User{
		UserService: services.User,
	}
	requireUserMw := middleware.RequireUser{
		User: userMw,
	}

	r.Handle("/", staticC.Home).Methods("GET")
	r.Handle("/contact", staticC.Contact).Methods("GET")
	r.HandleFunc("/signup", usersC.New).Methods("GET")
	r.HandleFunc("/signup", usersC.Create).Methods("POST")
	r.HandleFunc("/login", usersC.Show).Methods("GET")
	r.HandleFunc("/login", usersC.Login).Methods("POST")
	r.HandleFunc("/logout", requireUserMw.ApplyFn(usersC.Logout)).Methods("POST")
	r.Handle("/forgot", usersC.ForgotPwView).Methods("GET")
	r.HandleFunc("/forgot", usersC.InitiateReset).Methods("POST")
	r.HandleFunc("/reset", usersC.ResetPw).Methods("GET")
	r.HandleFunc("/reset", usersC.CompleteReset).Methods("POST")

	// OAuth Routes
	r.HandleFunc("/oauth/{service:[a-z]+}/connect", requireUserMw.ApplyFn(oauthsC.Connect))
	r.HandleFunc("/oauth/{service:[a-z]+}/callback", requireUserMw.ApplyFn(oauthsC.Callback))
	r.HandleFunc("/oauth/{service:[a-z]+}/test", requireUserMw.ApplyFn(oauthsC.DropboxTest))

	// Assets
	assetHandler := http.FileServer(http.Dir("./assets/"))
	assetHandler = http.StripPrefix("/assets/", assetHandler)
	r.PathPrefix("/assets/").Handler(assetHandler)

	// Image routes
	imageHandler := http.FileServer(http.Dir("./images/"))
	r.PathPrefix("/images/").Handler(http.StripPrefix("/images/", imageHandler))

	// Gallery routes
	r.Handle("/galleries", requireUserMw.ApplyFn(galleriesC.Index)).Methods("Get").Name("index_gallery")
	r.Handle("/galleries/new", requireUserMw.Apply(galleriesC.New)).Methods("Get")
	r.HandleFunc("/galleries", requireUserMw.ApplyFn(galleriesC.Create)).Methods("POST")
	r.HandleFunc("/galleries/{id:[0-9]+}/edit", requireUserMw.ApplyFn(galleriesC.Edit)).Methods("GET").Name("edit_gallery")
	r.HandleFunc("/galleries/{id:[0-9]+}/update", requireUserMw.ApplyFn(galleriesC.Update)).Methods("POST")
	r.HandleFunc("/galleries/{id:[0-9]+}/delete", requireUserMw.ApplyFn(galleriesC.Delete)).Methods("POST")
	r.HandleFunc("/galleries/{id:[0-9]+}/images", requireUserMw.ApplyFn(galleriesC.ImageUpload)).Methods("POST")
	r.HandleFunc("/galleries/{id:[0-9]+}/images/{filename}/delete", requireUserMw.ApplyFn(galleriesC.ImageDelete)).Methods("POST")
	r.HandleFunc("/galleries/{id:[0-9]+}", galleriesC.Show).Methods("GET").Name("show_gallery")

	fmt.Println("Server is listening on port " + serverPort)
	http.ListenAndServe(":"+serverPort, csrfMw(userMw.Apply(r)))
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
