package auth

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/Joematpal/test-api/src/v1/users"
	"github.com/Joematpal/test-api/src/v1/utils"
	jwt "github.com/dgrijalva/jwt-go"
	uuid "github.com/satori/go.uuid"
)

// SetToken is middleware that sets the user's token.
func SetToken() utils.Adapter {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			uCtx := r.Context().Value(CtxKey("userid"))
			u := uCtx.(users.User)

			expireToken := time.Now().Add(time.Hour * 8).Unix()
			expireCookie := time.Now().Add(time.Hour * 8)

			claims := Session{
				u.Username,
				u.ID,
				u.Email,
				jwt.StandardClaims{
					ExpiresAt: expireToken,
					//TODO: what does the issuer do?
					Issuer: "localhost:8000",
				},
			}

			token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
			signedToken, _ := token.SignedString([]byte(os.Getenv("TOKEN_SECRET")))

			cookie := http.Cookie{
				Name:     "Auth",
				Value:    signedToken,
				Expires:  expireCookie, // 30 min
				HttpOnly: true,
				Path:     "/",
				Domain:   os.Getenv("SERVER_DOMAIN"),
				Secure:   true,
			}

			http.SetCookie(w, &cookie)

			utils.Respond(w, r, http.StatusOK, u.Public(), nil)
			h.ServeHTTP(w, r)
		})
	}
}

// CheckUser is the middleware
func CheckUser(db *sql.DB) utils.Adapter {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			u := users.User{}
			if err := utils.Decode(r, &u); err != nil {
				fmt.Println("err", err)
				utils.Respond(w, r, http.StatusBadRequest, nil, "err at decode")
				return
			}

			if err := u.CheckPass(db); err != nil {
				utils.Respond(w, r, http.StatusBadRequest, "what", "The Username or Password provided could not be authenticated.")
				return
			}

			match := utils.CheckPasswordHash(u.Password, u.PasswordHash)
			if !match {
				utils.Respond(w, r, http.StatusBadRequest, nil, "Wrong password")
				return
			}

			ctx := context.WithValue(r.Context(), CtxKey("userid"), u)
			h.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// SignUp creates a newUser
func SignUp(db *sql.DB) utils.Adapter {

	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer db.Close()
			u := users.User{}

			if err := utils.Decode(r, &u); err != nil {
				utils.Respond(w, r, http.StatusBadRequest, nil, "err at decode")
				return
			}

			hash, err := utils.HashPassword(u.Password)
			if err != nil {
				//TODO: make sure that the front end and the backend are communicating properly on duplicate information.
				utils.Respond(w, r, http.StatusPartialContent, nil, err)
				return
			}
			u.PasswordHash = hash
			u.ID = uuid.NewV4()
			if err := u.CreateUser(db); err != nil {
				utils.Respond(w, r, http.StatusPartialContent, nil, err)
				return
			}
			h.ServeHTTP(w, r)
		})
	}
}

// PassConfirm checks the password and passwordConfirm.
func PassConfirm() utils.Adapter {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u := users.NewUser{}

			if err := utils.Decode(r, &u); err != nil {
				utils.Respond(w, r, http.StatusBadRequest, nil, err)
				return
			}
			if u.Password != u.PasswordConfirm {
				utils.Respond(w, r, http.StatusPartialContent, nil, "password and password confirm do not match")
				return
			}

			h.ServeHTTP(w, r)
		})
	}
}

// RemoveToken from
func RemoveToken() utils.Adapter {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		})
	}
}

// Validate checks if the token on the request has a uuid that matches the db.
func Validate(db *sql.DB) utils.Adapter {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie("Auth")
			if err != nil {
				utils.Respond(w, r, http.StatusBadRequest, nil, err)
				return
			}

			token, e := jwt.ParseWithClaims(cookie.Value, &Session{},
				func(token *jwt.Token) (interface{}, error) {
					if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
						return nil, fmt.Errorf("Unexpected signing method")
					}
					return []byte(os.Getenv("TOKEN_SECRET")), nil
				})
			if e != nil {
				utils.Respond(w, r, http.StatusBadRequest, nil, e)
				return
			}

			claims, ok := token.Claims.(*Session)
			if !ok && !token.Valid {

				utils.Respond(w, r, http.StatusBadRequest, nil, "please provide a token")
				return
			}

			u := users.User{ID: claims.ID}

			if err := u.CheckID(db); err != nil {
				utils.Respond(w, r, http.StatusBadRequest, nil, err)
				return
			}

			ctx := context.WithValue(r.Context(), CtxKey("userid"), u)

			h.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
