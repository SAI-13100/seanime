package core

import (
	"context"
	"errors"
	"github.com/seanime-app/seanime/internal/anilist"
	"github.com/seanime-app/seanime/internal/models"
)

// GetAnilistCollection returns the user's Anilist collection if it in the cache, otherwise it queries Anilist for the user's collection.
// When bypassCache is true, it will always query Anilist for the user's collection
func (a *App) GetAnilistCollection(bypassCache bool) (*anilist.AnimeCollection, error) {

	// Get Anilist Collection from App if it exists
	if !bypassCache && a.anilistCollection != nil {
		return a.anilistCollection, nil
	}

	return a.RefreshAnilistCollection()

}

// RefreshAnilistCollection queries Anilist for the user's collection
func (a *App) RefreshAnilistCollection() (*anilist.AnimeCollection, error) {

	// If the account is nil, return false
	if a.account == nil {
		return nil, errors.New("no account was found")
	}

	// Else, get the collection from Anilist
	collection, err := a.AnilistClient.AnimeCollection(context.Background(), &a.account.Username)
	if err != nil {
		return nil, err
	}

	// Save the collection to App
	a.anilistCollection = collection

	return collection, nil
}

func (a *App) GetAccount() (*models.Account, error) {

	if a.account == nil {
		return nil, errors.New("no account was found")
	}

	if a.account.Username == "" {
		return nil, errors.New("no username was found")
	}

	if a.account.Token == "" {
		return nil, errors.New("no token was found")
	}

	return a.account, nil

}
