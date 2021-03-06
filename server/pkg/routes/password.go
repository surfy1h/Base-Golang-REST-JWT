package routes

import (
	"fmt"
	"github.com/dgrijalva/jwt-go"
	"github.com/diogox/REST-JWT/server"
	"github.com/diogox/REST-JWT/server/pkg/models"
	"github.com/diogox/REST-JWT/server/pkg/token"
	"github.com/labstack/echo"
	"golang.org/x/crypto/bcrypt"
	"net/http"
)

func sendPasswordResetEmail(c echo.Context) error {
	return sendPasswordResetEmailHandler(c, db, tokenWhitelist, emailService)
}

func sendPasswordResetEmailHandler(c echo.Context, db server.DB, whitelist server.Whitelist, emailService server.EmailService) error {
	// Get context
	ctx := c.Request().Context()

	// Get logger
	logger := c.Logger()

	// Request body
	var req struct {
		Email string `json:"email" validate:"email,required"`
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

	// Get user
	reqUser, err := db.GetUserByEmail(ctx, req.Email)
	if err != nil {
		// TODO: Maybe it's more helpful to specify that the username doesn't exist?
		// No user found
		return c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Message: "No user found with that email!",
		})
	}

	// Check if email is verified
	if !reqUser.IsEmailVerified {
		return c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Message: "Email not verified!",
		})
	}

	// Generate encoded token and send it as response.
	opts := token.PasswordResetTokenOptions{
		JWTSecret:         jwtSecret,
		UserId:            reqUser.ID,
		DurationInMinutes: authTokenDurationInMinutes,
	}

	resetToken, err := token.NewPasswordResetToken(opts)
	if err != nil {
		logger.Error(err.Error())

		return c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Message: err.Error(),
		})
	}

	logger.Info("Reset password Token: " + resetToken)

	// Create `User` response
	user := models.User{
		Email:    reqUser.Email,
		Username: reqUser.Username,
	}

	// Send verification email
	err = emailService.SendEmail(user, models.NewEmail{
		Subject: "Registration",
		Message: fmt.Sprintf("%s, you have requested a password reset: %s", user.Username, AppUrl+"/reset-password/"+resetToken),
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Message: "Failed to send password reset email!\n" + err.Error(),
		})
	}

	// Add to token whitelist
	err = whitelist.SetResetPasswordTokenByUserID(reqUser.ID, resetToken, authTokenDurationInMinutes)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Message: "Failed to whitelist token!",
		})
	}

	return c.String(http.StatusOK, "Password reset email sent!")
}

func resetPassword(c echo.Context) error {
	return resetPasswordHandler(c, tokenWhitelist, loginBlacklist, db)
}

func resetPasswordHandler(c echo.Context, whitelist server.Whitelist, loginBlacklist server.Blacklist, db server.DB) error {
	// Get context
	ctx := c.Request().Context()
	logger := c.Logger()

	// Get token
	tokenString := c.Param("token")

	// Parse token
	tokn, err := jwt.Parse(tokenString, func(token *jwt.Token) (i interface{}, e error) {
		return jwtSecret, nil
	})
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Message: "Invalid token!",
		})
	}

	// Check if the token is valid
	if !token.AssertAndValidate(tokn, token.PasswordResetToken) {
		return c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Message: "Invalid token!",
		})
	}

	// Get user id associated with token
	claims := tokn.Claims.(jwt.MapClaims)
	userId, _ := claims["user_id"].(string)

	// Make sure token hasn't been used already
	_, err = whitelist.GetResetPasswordTokenByUserID(userId)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Message: "Token has been used!",
		})
	}

	// Get new password sent in the body of the request
	var newPasswordReq struct {
		Password string `json:"password" validate:"isValidPassword,required"`
	}

	// Get POST body
	err = c.Bind(&newPasswordReq)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Message: "Invalid request body!",
		})
	}

	// Validate request
	err = c.Validate(newPasswordReq)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Message: err.Error(),
		})
	}

	// Salt and hash the password using the bcrypt algorithm
	// The second argument is the cost of hashing, which we arbitrarily set as 8
	// (this value can be more or less, depending on the computing power you wish to utilize)
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPasswordReq.Password), 8)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Message: err.Error(),
		})
	}

	// Get user
	user, err := db.GetUserByID(ctx, userId)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Message: err.Error(),
		})
	}

	// Update user to have the new password
	_, err = db.UpdateUserByID(ctx, userId, &models.User{
		ID:              user.ID,
		Email:           user.Email,
		Username:        user.Username,
		Password:        string(hashedPassword),
		IsEmailVerified: user.IsEmailVerified,
		IsPaidUser:      user.IsPaidUser,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Message: err.Error(),
		})
	}

	// Remove token from the whitelist
	_, err = whitelist.DelResetPasswordTokenByUserID(userId)
	if err != nil {
		logger.Error("Failed to remove reset-password token from whitelist: " + err.Error())
	}

	// Unlock the account, if locked
	err = loginBlacklist.ResetFailedLoginCountByUserID(userId)
	if err != nil {
		logger.Error("Failed to reset the 'failed login' counter for the user ID: " + userId)
	}

	return c.String(http.StatusOK, "Password changed successfully!")
}
