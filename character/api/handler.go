package api

import (
	"net/http"
	"strconv"

	"ai-agent-character-demo/backend/character/models"
	"ai-agent-character-demo/backend/character/service"

	"github.com/gin-gonic/gin"
)

type CharacterHandler struct {
	service *service.CharacterService
}

func NewCharacterHandler(service *service.CharacterService) *CharacterHandler {
	return &CharacterHandler{service: service}
}

func (h *CharacterHandler) CreateCharacter(c *gin.Context) {
	var character models.Character
	if err := c.ShouldBindJSON(&character); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.service.CreateCharacter(&character); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, character)
}

func (h *CharacterHandler) GetCharacterByID(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid character ID"})
		return
	}
	character, err := h.service.GetCharacterByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Character not found"})
		return
	}
	c.JSON(http.StatusOK, character)
}

func (h *CharacterHandler) GetAllCharacters(c *gin.Context) {
	characters, err := h.service.GetAllCharacters()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if characters == nil {
		characters = []models.Character{}
	}
	c.JSON(http.StatusOK, characters)
}
