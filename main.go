package main

import (
	"bytes"
	"fmt"
	"log"
	"sync"
)

const (
	foundInCache    = "found in cache!"
	notFoundInCache = "not found in cache"
	foundInDB       = "found in db!!"
	notFoundInDB    = "not found in db"
	appIsStarted    = "app is started!"
)

type User struct {
	Name string
}

type UserRepo struct {
	o         sync.Once
	db        map[int]User
	isCacheSM bool
	cacheSM   sync.Map
	rwm       sync.RWMutex
	cacheRWM  map[int]User
	logger    *log.Logger
}

func (u *UserRepo) getFromCache(id int) (User, bool) {
	if u.isCacheSM {
		user, ok := u.cacheSM.Load(id)
		if !ok {
			u.logger.Println(notFoundInCache)
			return User{}, false
		}
		u.logger.Println(foundInCache)
		return user.(User), true
	}

	u.rwm.RLock()
	user, ok := u.cacheRWM[id]
	u.rwm.RUnlock()
	if !ok {
		u.logger.Println(notFoundInCache)
		return User{}, false
	}

	u.logger.Println(foundInCache)

	return user, true
}

func (u *UserRepo) storeInCache(id int, user User) {
	if u.isCacheSM {
		u.cacheSM.Store(id, user)
		return
	}

	u.rwm.Lock()
	u.cacheRWM[id] = user
	u.rwm.Unlock()
}

func (u *UserRepo) Get(id int) (User, bool) {
	if v, ok := u.getFromCache(id); ok {
		return v, true
	}

	user, ok := u.db[id]
	if !ok {
		u.logger.Println(notFoundInDB)
		return User{}, false
	}

	u.storeInCache(id, user)

	u.logger.Println(foundInDB)

	return user, true
}

func (u *UserRepo) Store(id int, user User) {
	u.db[id] = user
	u.storeInCache(id, user)
}

func (u *UserRepo) Init(isCacheSM bool, logger *log.Logger) {
	u.isCacheSM = isCacheSM
	u.logger = logger
	u.o.Do(u.doInit)
}

func (u *UserRepo) doInit() {
	u.db = make(map[int]User)
	u.cacheRWM = make(map[int]User)
}

type UserService struct {
	repo *UserRepo
}

func (u *UserService) Init(r *UserRepo) {
	u.repo = r
}

func (u *UserService) Get(id int) (User, bool) {
	return u.repo.Get(id)
}

func (u *UserService) Store(id int, user User) {
	u.repo.Store(id, user)
}

type UserServer struct {
	service *UserService
}

func (u *UserServer) Init(service *UserService) {
	u.service = service
}

func (u *UserServer) Get(id int) (User, bool) {
	return u.service.Get(id)
}

func (u *UserServer) Store(id int, user User) {
	u.service.Store(id, user)
}

type App struct {
	o         sync.Once
	buf       bytes.Buffer
	logger    *log.Logger
	UserS     UserServer
	isCacheSM bool
}

func (a *App) Init(isCacheSM bool) {
	a.isCacheSM = isCacheSM
	a.o.Do(a.doInit)
}

func (a *App) doInit() {
	a.logger = log.New(&a.buf, "", log.LstdFlags)
	userRepo := UserRepo{}
	userRepo.Init(a.isCacheSM, a.logger)

	userService := UserService{}
	userService.Init(&userRepo)

	userServer := UserServer{}
	userServer.Init(&userService)
	a.UserS = userServer

	a.logger.Println(appIsStarted)
}

func (a *App) Println() {
	fmt.Println(a.buf.String())
}

func CreateApp(isCacheSM bool) *App {
	app := App{}
	app.Init(isCacheSM)

	return &app
}

func ScenarioSyncMapCache() {
	app := CreateApp(true)
	Scenario(app)
}

func ScenarioRWMutexCache() {
	app := CreateApp(false)
	Scenario(app)
}

func Scenario(app *App) {
	v, ok := app.UserS.Get(1)
	app.logger.Println(v, ok)
	app.UserS.Store(1, User{Name: "Ivan"})
	v, ok = app.UserS.Get(1)
	app.logger.Println(v, ok)
	v, ok = app.UserS.Get(1)
	app.logger.Println(v, ok)
	app.Println()
}

func main() {
	ScenarioSyncMapCache()
	ScenarioRWMutexCache()
}
