package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
)

// Authenticator verify JWT + gameSessionId, khớp với pattern NestJS WsJwtGuard.
//
// Flow giống NestJS handleConnection:
// 1. Parse JWT bằng JWT_SECRET share với NestJS.
// 2. Lấy userId từ JWT payload.
// 3. Đọc Redis `user:${userId}:gameSession` — phải khớp với gameSessionId client gửi lên.
// 4. Nếu sai → reject.
//
// Tại sao 2 lớp (JWT + gameSession)?
// JWT chỉ chứng minh "ai" — gameSession chứng minh "session nào còn hợp lệ".
// User login lại trên thiết bị khác → NestJS đổi gameSession trong Redis → JWT cũ vẫn valid
// nhưng gameSession không khớp → reject. Đây là pattern revocation cho JWT (vốn không có built-in).
type Authenticator struct {
	jwtSecret []byte
	redis     *redis.Client
}

func NewAuthenticator(jwtSecret string, rdb *redis.Client) *Authenticator {
	return &Authenticator{
		jwtSecret: []byte(jwtSecret),
		redis:     rdb,
	}
}

// AuthResult - kết quả verify thành công.
type AuthResult struct {
	UserID int32
	Role   string
}

var (
	ErrInvalidToken   = errors.New("invalid token")
	ErrInvalidSession = errors.New("invalid game session")
	ErrUserIDMismatch = errors.New("userID in JWT doesn't match handshake")
)

// Verify JWT và gameSessionId. Trả về userID + role nếu OK.
//
// claimedUserID là userID client gửi trong handshake — phải khớp với JWT.
// Tại sao có 2 nguồn? Để Go service không phải parse JWT mỗi packet — chỉ parse 1 lần lúc handshake.
func (a *Authenticator) Verify(ctx context.Context, token string, gameSessionID string, claimedUserID int32) (*AuthResult, error) {
	// Parse JWT.
	parsed, err := jwt.Parse(token, func(t *jwt.Token) (any, error) {
		// NestJS dùng HS256 mặc định. Nếu sau này đổi sang RS256 thì phải verify khác.
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return a.jwtSecret, nil
	})
	if err != nil || !parsed.Valid {
		return nil, ErrInvalidToken
	}

	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return nil, ErrInvalidToken
	}

	// JWT của NestJS có field "userId" (camelCase từ payload).
	// JSON number → float64 trong Go.
	userIDFloat, ok := claims["userId"].(float64)
	if !ok {
		return nil, ErrInvalidToken
	}
	userID := int32(userIDFloat)

	// Verify userID khớp với claimedUserID.
	if userID != claimedUserID {
		return nil, ErrUserIDMismatch
	}

	// Verify gameSession khớp Redis.
	currentSession, err := a.redis.Get(ctx, fmt.Sprintf("user:%d:gameSession", userID)).Result()
	if err != nil {
		return nil, ErrInvalidSession
	}
	if currentSession != gameSessionID {
		return nil, ErrInvalidSession
	}

	// Lấy role (optional).
	role, _ := claims["role"].(string)

	return &AuthResult{
		UserID: userID,
		Role:   role,
	}, nil
}
