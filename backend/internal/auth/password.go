package auth

import "golang.org/x/crypto/bcrypt"

const bcryptCost = 10

// HashPassword hashes with bcrypt cost 10 (contract §3).
func HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	return string(b), err
}

// CheckPassword reports whether the password matches the stored hash.
func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
