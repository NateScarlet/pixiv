package client

import (
	"net/url"
	"strconv"
)

// SiteURL build url for path on pixiv site
func SiteURL(path string) *url.URL {
	return &url.URL{
		Scheme: "https",
		Host:   "www.pixiv.net",
		Path:   path,
	}
}

// `/ajax`

// APIArtworkSearchURL returns url for artwork search api call.
func APIArtworkSearchURL(query string, page int) *url.URL {
	u := SiteURL("/ajax/search/artworks/" + query)
	q := u.Query()
	if page != 1 {
		q.Set("p", strconv.Itoa(page))
	}
	u.RawQuery = q.Encode()
	return u
}

// APIIllustURL returns url for single illust api query.
func APIIllustURL(id string) *url.URL {
	return SiteURL("/ajax/illust/" + id)
}

// APIIllustPagesURL returns url for illust pages api query.
func APIIllustPagesURL(id string) *url.URL {
	return SiteURL("/ajax/illust/" + id + "/pages")
}

// APINovelSearchURL returns url for novel search api query.
func APINovelSearchURL(query string, page int) *url.URL {
	u := SiteURL("/ajax/search/novels/" + query)
	q := u.Query()
	if page != 1 {
		q.Set("p", strconv.Itoa(page))
	}
	u.RawQuery = q.Encode()
	return u
}

// APINovelURL returns url for single novel api query.
func APINovelURL(id string) *url.URL {
	return SiteURL("/ajax/novel/" + id)
}

// APIUserURL returns url for single user api query.
func APIUserURL(id string) *url.URL {
	return SiteURL("/ajax/user/" + id)
}

// APIFullUserURL returns url for single user api query (full fields).
func APIFullUserURL(id string) *url.URL {
	u := APIFullUserURL(id)
	q := u.Query()
	q.Set("full", "1")
	u.RawQuery = q.Encode()
	return u
}

// `/`

// ArtworkURL returns url for given artwork id.
func ArtworkURL(id string) *url.URL {
	return SiteURL("/artworks/" + id)
}

// TagURL returns url for pixiv tag.
func TagURL(tag string) *url.URL {
	return SiteURL("/tags/" + tag)
}

// UserURL returns url for given user id.
func UserURL(id string) *url.URL {
	u := SiteURL("/member.php")
	q := u.Query()
	q.Set("id", id)
	u.RawQuery = q.Encode()
	return u
}

// NovelURL returns url for given novel id.
func NovelURL(id string) *url.URL {
	u := SiteURL("/novel/show.php")
	q := u.Query()
	q.Set("id", id)
	u.RawQuery = q.Encode()
	return u
}

// NovelSeriesURL returns url for given novel series id.
func NovelSeriesURL(id string) *url.URL {
	return SiteURL("/novel/series/" + id)
}

// ArtworkSearchURL returns url for search artworks.
func ArtworkSearchURL(query string) *url.URL {
	return SiteURL("/tags/" + query + "/artworks")
}

// NovelSearchURL returns url for search novels.
func NovelSearchURL(query string) *url.URL {
	return SiteURL("/tags/" + query + "/novels")
}
