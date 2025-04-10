package main

import (
	"encoding/json"
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/mux"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
)

type APIServer struct {
	listenAddr string
	store      Storage
}

func NewAPIServer(listenAddr string, store Storage) *APIServer {
	return &APIServer{
		listenAddr: listenAddr,
		store:      store,
	}
}

func (s *APIServer) Run() {
	router := mux.NewRouter()

	router.HandleFunc("/login", makeHTTPHandleFunc(s.handleLogin))
	router.HandleFunc("/user", makeHTTPHandleFunc(s.handleUser))
	router.HandleFunc("/user/{id}", withJWTAuth(makeHTTPHandleFunc(s.handleUserByID)))
	router.HandleFunc("/transfer", makeHTTPHandleFunc(s.handleTransfer))

	log.Println("JSON API server running on pswitcht: ", s.listenAddr)

	http.ListenAndServe(s.listenAddr, router)
}

func (s *APIServer) handleLogin(w http.ResponseWriter, r *http.Request) error {
	if r.Method != "POST" {
		return fmt.Errorf("method not allowed %s", r.Method)
	}

	loginRequest := new(LoginRequest)
	if err := json.NewDecoder(r.Body).Decode(loginRequest); err != nil {
		return err
	}

	user, err := s.store.GetUserByCPF(loginRequest.CPF)
	if err != nil {
		return err
	}

	if IsPasswordIncorrect(loginRequest.Password, user.Password) {
		return WriteJSON(w, http.StatusUnauthorized, map[string]string{"message": "CPF ou senha incorretos"})
	}

	token, err := createJWT(user)
	if err != nil {
		return err
	}

	return WriteJSON(w, http.StatusOK, map[string]string{"authToken": token, "message": "logado com sucesso"})
}

func (s *APIServer) handleUser(w http.ResponseWriter, r *http.Request) error {
	switch r.Method {
	case "GET":
		return s.handleGetUser(w)
	case "POST":
		return s.handleCreateUser(w, r)
	default:
		return fmt.Errorf("method not allowed %s", r.Method)
	}
}

func (s *APIServer) handleUserByID(w http.ResponseWriter, r *http.Request) error {
	switch r.Method {
	case "GET":
		return s.handleGetUserByID(w, r)
	case "DELETE":
		return s.handleDeleteUserByID(w, r)
	default:
		return fmt.Errorf("method not allowed %s", r.Method)
	}
}

func (s *APIServer) handleGetUserByID(w http.ResponseWriter, r *http.Request) error {
	id, err := getID(r)
	if err != nil {
		return err
	}

	user, err := s.store.GetUserByID(id)
	if err != nil {
		return err
	}

	return WriteJSON(w, http.StatusOK, user)
}

func (s *APIServer) handleGetUser(w http.ResponseWriter) error {
	users, err := s.store.GetUsers()

	if err != nil {
		return err
	}

	return WriteJSON(w, http.StatusOK, users)
}

func (s *APIServer) handleCreateUser(w http.ResponseWriter, r *http.Request) error {
	createUserRequest := new(CreateUserRequest)

	user, err := NewUser(createUserRequest)
	if err != nil {
		return err
	}

	if err := s.store.CreateUser(user); err != nil {
		return err
	}

	return WriteJSON(w, http.StatusCreated, map[string]string{"message": "user created"})
}

func (s *APIServer) handleDeleteUserByID(w http.ResponseWriter, r *http.Request) error {
	id, err := getID(r)
	if err != nil {
		return err
	}

	if err := s.store.DeleteUser(id); err != nil {
		return err
	}

	return WriteJSON(w, http.StatusOK, map[string]int{"deleted": id})
}

func (s *APIServer) handleTransfer(w http.ResponseWriter, r *http.Request) error {
	transferReq := new(TransferRequest)
	if err := json.NewDecoder(r.Body).Decode(transferReq); err != nil {
		return err
	}
	defer r.Body.Close()

	return WriteJSON(w, http.StatusOK, transferReq)
}

func WriteJSON(w http.ResponseWriter, status int, v any) error {
	w.Header().Add("Content-Type", "aplication/json")
	w.WriteHeader(status)

	return json.NewEncoder(w).Encode(v)
}

func createJWT(user *User) (string, error) {
	claims := &jwt.MapClaims{
		"expiresAt":     3600,
		"userNumber": user.Number,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	return token.SignedString([]byte(os.Getenv("JWT_SECRET")))
}

func withJWTAuth(handlerFunc http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tokenString := treatAuthorizationHeader(r.Header.Get("Authorization"))
		_, err := validateJWT(tokenString)

		if err != nil {
			WriteJSON(w, http.StatusForbidden, ApiError{Error: "invalid token"})

			return
		}

		// claims := token.Claims.(jwt.MapClaims)
		// log.Println("Token: ", claims)
		handlerFunc(w, r)
	}
}

func treatAuthorizationHeader(authorizationHeader string) string {
	if strings.HasPrefix(authorizationHeader, "Bearer ") {
		return strings.TrimPrefix(authorizationHeader, "Bearer ")
	}

	return authorizationHeader
}

func validateJWT(tokenString string) (*jwt.Token, error) {
	return jwt.Parse(tokenString, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		return []byte(os.Getenv("JWT_SECRET")), nil
	})
}

type apiFunc func(http.ResponseWriter, *http.Request) error

type ApiError struct {
	Error string `json:"error"`
}

func makeHTTPHandleFunc(f apiFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := f(w, r); err != nil {
			WriteJSON(w, http.StatusBadRequest, ApiError{Error: err.Error()})
		}
	}
}
func getID(r *http.Request) (int, error) {
	idStr := mux.Vars(r)["id"]
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return id, fmt.Errorf("invalid id given %s", idStr)
	}

	return id, nil
}
