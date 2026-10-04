package main

import (
	"fmt"
	"net/http"
	"runtime"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/java1001/ginertia"
)

type User struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"createdAt"`
}

// UserForm is bound from the useForm payload. Field names in the
// validation errors come from the json tags, matching the Svelte form keys.
type UserForm struct {
	Name  string `json:"name" form:"name" binding:"required,min=2,max=50"`
	Email string `json:"email" form:"email" binding:"required,email"`
	Role  string `json:"role" form:"role" binding:"required,oneof=admin editor viewer"`
}

type handlers struct {
	store *store
	posts []Post
	sess  *session
}

type Post struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	Body  string `json:"body"`
}

func newPosts(n int) []Post {
	topics := []string{"Gin middleware", "GORM preload", "Svelte runes", "Vite HMR", "Inertia partial reloads",
		"Go generics", "PostgreSQL indexes", "WebSocket fan-out", "Flutter state", "SEO for SPAs"}
	posts := make([]Post, n)
	for i := range posts {
		posts[i] = Post{
			ID:    i + 1,
			Title: fmt.Sprintf("#%d · Notes on %s", i+1, topics[i%len(topics)]),
			Body:  "Short note body, rendered from a Go slice and streamed into the page as you scroll.",
		}
	}
	return posts
}

// feed: <InfiniteScroll data="posts"> on the client requests ?page=N and
// the new page's items are appended (or prepended when scrolling up).
func (h *handlers) feed(c *gin.Context) {
	const perPage = 15
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	page = max(page, 1)
	start := min((page-1)*perPage, len(h.posts))
	end := min(start+perPage, len(h.posts))

	ginertia.Render(c, "Feed", gin.H{
		"posts": ginertia.ScrollPage(h.posts[start:end], page, end < len(h.posts)).MatchOn("id"),
		"total": len(h.posts),
	})
}

func (h *handlers) home(c *gin.Context) {
	ginertia.ViewData(c, "title", "Home")
	ginertia.Render(c, "Home", gin.H{
		"goVersion": runtime.Version(),
		"serverTime": func() string { // evaluated only when sent
			return time.Now().Format("15:04:05")
		},
	})
}

func (h *handlers) usersIndex(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	page = max(page, 1)
	users, hasMore := h.store.page(page, 8)

	ginertia.ViewData(c, "title", "Users")
	ginertia.Render(c, "Users/Index", gin.H{
		// "Load more" appends to the list on the client (partial reload).
		"users":   ginertia.Merge(users).MatchOn("id"),
		"page":    page,
		"hasMore": hasMore,
		// Loaded right after the page shows up, in a second request.
		"stats": ginertia.Defer(func() any {
			time.Sleep(700 * time.Millisecond) // pretend it's a slow query
			return h.store.stats()
		}),
	})
}

func (h *handlers) usersCreate(c *gin.Context) {
	ginertia.Render(c, "Users/Form", gin.H{"user": nil})
}

func (h *handlers) usersStore(c *gin.Context) {
	var in UserForm
	if err := c.ShouldBind(&in); err != nil {
		ginertia.WithErrors(c, ginertia.ValidationErrors(err, in))
		ginertia.Back(c)
		return
	}
	if h.store.emailTaken(in.Email, 0) {
		ginertia.WithErrors(c, map[string]string{"email": "This email is already taken."})
		ginertia.Back(c)
		return
	}
	u := h.store.create(in)
	ginertia.Flash(c, "success", fmt.Sprintf("Created %s.", u.Name))
	ginertia.Redirect(c, "/users")
}

func (h *handlers) usersEdit(c *gin.Context) {
	u, ok := h.store.find(c.Param("id"))
	if !ok {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	ginertia.Render(c, "Users/Form", gin.H{"user": u})
}

func (h *handlers) usersUpdate(c *gin.Context) {
	u, ok := h.store.find(c.Param("id"))
	if !ok {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	var in UserForm
	if err := c.ShouldBind(&in); err != nil {
		ginertia.WithErrors(c, ginertia.ValidationErrors(err, in))
		ginertia.Back(c)
		return
	}
	if h.store.emailTaken(in.Email, u.ID) {
		ginertia.WithErrors(c, map[string]string{"email": "This email is already taken."})
		ginertia.Back(c)
		return
	}
	h.store.update(u.ID, in)
	ginertia.Flash(c, "success", "Saved.")
	// A plain c.Redirect(302) works too: the middleware turns it into 303.
	c.Redirect(http.StatusFound, "/users")
}

func (h *handlers) usersDestroy(c *gin.Context) {
	if u, ok := h.store.find(c.Param("id")); ok {
		h.store.delete(u.ID)
		ginertia.Flash(c, "success", fmt.Sprintf("Deleted %s.", u.Name))
	}
	ginertia.Back(c, "/users")
}

// ---------------------------------------------------------------------------
// In-memory store (swap for GORM in a real app)
// ---------------------------------------------------------------------------

type store struct {
	mu     sync.RWMutex
	users  []User
	nextID int
}

func newStore() *store {
	s := &store{nextID: 1}
	names := []string{"An", "Bình", "Chi", "Dũng", "Giang", "Hà", "Hùng", "Khoa", "Lan", "Linh",
		"Minh", "Nam", "Ngọc", "Phong", "Quân", "Sơn", "Thảo", "Trang", "Tuấn", "Vy"}
	roles := []string{"admin", "editor", "viewer", "viewer"}
	for i, n := range names {
		s.users = append(s.users, User{
			ID: s.nextID, Name: n, Email: fmt.Sprintf("user%d@example.com", s.nextID),
			Role: roles[i%len(roles)], CreatedAt: time.Now().Add(-time.Duration(i) * 24 * time.Hour),
		})
		s.nextID++
	}
	return s
}

func (s *store) page(page, size int) ([]User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	start := min((page-1)*size, len(s.users))
	end := min(start+size, len(s.users))
	return slices.Clone(s.users[start:end]), end < len(s.users)
}

func (s *store) find(id string) (User, bool) {
	n, _ := strconv.Atoi(id)
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, u := range s.users {
		if u.ID == n {
			return u, true
		}
	}
	return User{}, false
}

func (s *store) emailTaken(email string, exceptID int) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.ContainsFunc(s.users, func(u User) bool { return u.Email == email && u.ID != exceptID })
}

func (s *store) create(in UserForm) User {
	s.mu.Lock()
	defer s.mu.Unlock()
	u := User{ID: s.nextID, Name: in.Name, Email: in.Email, Role: in.Role, CreatedAt: time.Now()}
	s.nextID++
	s.users = append([]User{u}, s.users...)
	return u
}

func (s *store) update(id int, in UserForm) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.users {
		if s.users[i].ID == id {
			s.users[i].Name, s.users[i].Email, s.users[i].Role = in.Name, in.Email, in.Role
		}
	}
}

func (s *store) delete(id int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.users = slices.DeleteFunc(s.users, func(u User) bool { return u.ID == id })
}

func (s *store) stats() gin.H {
	s.mu.RLock()
	defer s.mu.RUnlock()
	byRole := map[string]int{}
	for _, u := range s.users {
		byRole[u.Role]++
	}
	return gin.H{"total": len(s.users), "byRole": byRole}
}
