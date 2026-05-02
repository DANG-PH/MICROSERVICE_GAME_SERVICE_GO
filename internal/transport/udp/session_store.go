package udp

// import (
// 	"crypto/rand"
// 	"sync"
// )

// // Token là 8 bytes random — key để lookup session.
// // 2^64 combinations, không thể brute-force trong lifetime của game session.
// // Nhỏ hơn JWT (thường 200+ bytes) → header overhead mỗi UDP packet chỉ 8 bytes.
// type Token [8]byte

// // Session lưu thông tin của 1 UDP session sau khi đã auth qua WS.
// type Session struct {
// 	UserID int32
// 	MapID  string // cập nhật khi player switchMap
// }

// // SessionStore map token → session, thread-safe.
// // Dùng sync.RWMutex thay vì sync.Map vì:
// //   - Key type là [8]byte (array), sync.Map dùng interface{} → boxing overhead
// //   - Biết trước access pattern: read nhiều (mỗi UDP packet), write ít (connect/disconnect)
// //   - RWMutex với RLock cho read → không block lẫn nhau
// type SessionStore struct {
// 	mu       sync.RWMutex
// 	sessions map[Token]*Session
// }

// func NewSessionStore() *SessionStore {
// 	return &SessionStore{
// 		sessions: make(map[Token]*Session),
// 	}
// }

// // Create tạo token mới cho userID sau khi WS handshake thành công.
// // Nếu userID đã có token cũ (reconnect) → tạo token mới, token cũ tự vô hiệu
// // vì map không còn entry đó nữa.
// func (s *SessionStore) Create(userID int32) (Token, error) {
// 	var tok Token
// 	if _, err := rand.Read(tok[:]); err != nil {
// 		return tok, err
// 	}

// 	s.mu.Lock()
// 	// Xóa token cũ nếu user reconnect — tránh leak entry cũ
// 	s.deleteByUserIDLocked(userID)
// 	s.sessions[tok] = &Session{UserID: userID}
// 	s.mu.Unlock()

// 	return tok, nil
// }

// // Get lookup session theo token — gọi mỗi UDP packet.
// // RLock vì chỉ đọc, không block các goroutine read khác.
// func (s *SessionStore) Get(tok Token) (*Session, bool) {
// 	s.mu.RLock()
// 	sess, ok := s.sessions[tok]
// 	s.mu.RUnlock()
// 	return sess, ok
// }

// // Delete xóa session khi WS disconnect.
// // Dùng userID thay vì token vì Hub biết userID, không giữ token.
// func (s *SessionStore) Delete(userID int32) {
// 	s.mu.Lock()
// 	s.deleteByUserIDLocked(userID)
// 	s.mu.Unlock()
// }

// // UpdateMapID gọi khi player switchMap — giữ session sync với Hub.
// func (s *SessionStore) UpdateMapID(tok Token, mapID string) {
// 	s.mu.Lock()
// 	if sess, ok := s.sessions[tok]; ok {
// 		sess.MapID = mapID
// 	}
// 	s.mu.Unlock()
// }

// // deleteByUserIDLocked xóa entry theo userID — phải giữ write lock khi gọi.
// func (s *SessionStore) deleteByUserIDLocked(userID int32) {
// 	for tok, sess := range s.sessions {
// 		if sess.UserID == userID {
// 			delete(s.sessions, tok)
// 			return
// 		}
// 	}
// }
