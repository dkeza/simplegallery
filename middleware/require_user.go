package middleware

import (
	"fmt"
	"lenslocked/models"
	"net/http"
)

type RequireUser struct {
	models.UserService
}

func (mw *RequireUser) Apply(next http.Handler) http.HandlerFunc {
	return mw.ApplyFn(next.ServeHTTP)
}

func (mw *RequireUser) ApplyFn(next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		cookie, err := r.Cookie("remember_token")
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		cookievalue := cookie.Value
		user, err := mw.UserService.ByRemember(cookievalue)
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		fmt.Println("User found: ", user)

		next(w, r)

	})
}
