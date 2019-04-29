package routes

import (
	"github.com/dgrijalva/jwt-go"
	"github.com/diogox/REST-JWT/server/pkg/models"
	"github.com/diogox/REST-JWT/server/pkg/models/auth"
	"github.com/diogox/REST-JWT/server/pkg/token"
	"github.com/labstack/echo"
	"net/http"
	"time"
)

func refreshToken(c echo.Context) error {

	// Get logger
	logger := c.Logger()

	// Request body
	var req struct {
		RefreshToken string `json:"refresh_token" validate:"required"`
	}

	// Get POST body
	err := c.Bind(&req)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Message: "Invalid request body!",
		})
	}

	// Validate request
	err = c.Validate(req)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Message: err.Error(),
		})
	}

	// Get token from string
	refreshToken, err := jwt.Parse(req.RefreshToken, func(token *jwt.Token) (i interface{}, e error) {
		return jwtSecret, nil
	})

	// Get expiration time
	claims := refreshToken.Claims.(jwt.MapClaims)
	expiration, _ := claims["exp"].(int64)
	userId, _ := claims["user_id"].(string)

	// Make sure the token hasn't expired
	if time.Unix(expiration, 0).Sub(time.Now()) > 0*time.Second {
		return c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Message: "Token still valid for sufficient time. Try again later!",
		})
	}

	// Get from whitelist
	previousRefreshToken, err := refreshTokenWhitelist.Get(userId)
	if err != nil {

		// Not found (most likely)
		return c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Message: "Invalid Token!",
		})
	}

	// The given refresh_token must match the previous token, otherwise it's invalid
	if previousRefreshToken != req.RefreshToken {

		// Not found (most likely)
		return c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Message: "Invalid Token!",
		})
	}

	// Generate encoded token and send it as response.
	refreshTokenOpts := token.RefreshTokenOptions{
		JWTSecret:         jwtSecret,
		DurationInMinutes: refreshTokenDurationInMinutes,
		UserId:            userId,
	}
	newRefreshTokenStr, err := token.NewRefreshTokenToken(refreshTokenOpts)
	if err != nil {
		logger.Error(err.Error())

		return c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Message: err.Error(),
		})
	}

	// TODO: Refactor repetitive code into smaller functions

	// Add new token to whitelist (Replaces previous, if it exists)
	err = refreshTokenWhitelist.Set(userId, newRefreshTokenStr, refreshTokenDurationInMinutes)
	if err != nil {

		// Already exists (most likely)
		return c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Message: "Token already exists!",
		})
	}

	// Generate encoded token and send it as response.
	authTokenOpts := token.AuthTokenOptions{
		JWTSecret:         jwtSecret,
		DurationInMinutes: authTokenDurationInMinutes,
		UserID:            userId, // TODO: !!!!IMPORTANT!!!! Need to change auth token's options to take user id instead
	}
	newAuthtokenStr, err := token.NewAuthToken(authTokenOpts)
	if err != nil {
		logger.Error(err.Error())

		return c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Message: err.Error(),
		})
	}

	// Return new token
	return c.JSON(http.StatusOK, auth.LoginResponse{
		AuthToken:                   newAuthtokenStr, // TODO: Probably should return different struct (don't need this field)
		RefreshToken:                newRefreshTokenStr,
		ExpirationIntervalInMinutes: authTokenDurationInMinutes, // TODO: Is this necessary?
	})
}
