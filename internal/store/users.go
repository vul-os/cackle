package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// User is a Cackle account. PasswordHash is an argon2id PHC string produced
// by internal/auth — store never hashes or verifies passwords itself.
type User struct {
	ID              string
	Email           string
	PasswordHash    string
	Name            string
	CreatedAt       time.Time
	EmailVerifiedAt *time.Time
}

// CreateUser inserts a new user. If ID or CreatedAt are zero they are
// populated (ID via NewID, CreatedAt via time.Now).
func (s *Store) CreateUser(ctx context.Context, u *User) error {
	if u.ID == "" {
		u.ID = NewID()
	}
	if u.CreatedAt.IsZero() {
		u.CreatedAt = time.Now()
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO users (id, email, password_hash, name, created_at, email_verified_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		u.ID, u.Email, u.PasswordHash, u.Name, timeToText(u.CreatedAt), nullTimeToText(u.EmailVerifiedAt),
	)
	if err != nil {
		return fmt.Errorf("store: create user: %w", err)
	}
	return nil
}

// GetUserByID looks up a user by primary key. Returns ErrNotFound if absent.
func (s *Store) GetUserByID(ctx context.Context, id string) (*User, error) {
	return s.scanUser(s.db.QueryRowContext(ctx, `
		SELECT id, email, password_hash, name, created_at, email_verified_at
		FROM users WHERE id = ?`, id))
}

// GetUserByEmail looks up a user by email (case-sensitive; callers should
// normalise case before calling). Returns ErrNotFound if absent.
func (s *Store) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	return s.scanUser(s.db.QueryRowContext(ctx, `
		SELECT id, email, password_hash, name, created_at, email_verified_at
		FROM users WHERE email = ?`, email))
}

func (s *Store) scanUser(row *sql.Row) (*User, error) {
	var u User
	var createdAt string
	var emailVerifiedAt sql.NullString

	err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Name, &createdAt, &emailVerifiedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: scan user: %w", err)
	}

	if u.CreatedAt, err = textToTime(createdAt); err != nil {
		return nil, fmt.Errorf("store: parse user created_at: %w", err)
	}
	if u.EmailVerifiedAt, err = textToNullTime(emailVerifiedAt); err != nil {
		return nil, fmt.Errorf("store: parse user email_verified_at: %w", err)
	}
	return &u, nil
}

// UpdateUserPassword replaces a user's password hash (e.g. after a reset).
func (s *Store) UpdateUserPassword(ctx context.Context, userID, passwordHash string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE users SET password_hash = ? WHERE id = ?`, passwordHash, userID)
	if err != nil {
		return fmt.Errorf("store: update user password: %w", err)
	}
	return rowsAffectedOrNotFound(res)
}

// MarkEmailVerified sets email_verified_at for a user.
func (s *Store) MarkEmailVerified(ctx context.Context, userID string, at time.Time) error {
	res, err := s.db.ExecContext(ctx, `UPDATE users SET email_verified_at = ? WHERE id = ?`, timeToText(at), userID)
	if err != nil {
		return fmt.Errorf("store: mark email verified: %w", err)
	}
	return rowsAffectedOrNotFound(res)
}

func rowsAffectedOrNotFound(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: rows affected: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
