package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/cookiejar"
	urlpkg "net/url"
	"regexp"
	"strings"

	"github.com/dimchansky/utfbom"
	"github.com/tidwall/gjson"

	"github.com/hr3lxphr6j/ctfile/utils"
)

const (
	apiEndpoint = "https://webapi.400gb.com"
	origin      = "https://545c.com"
)

type Client struct {
	hc      *http.Client
	isLogin bool
}

func NewClient() *Client {
	jar, err := cookiejar.New(nil)
	if err != nil {
		panic(err)
	}
	return &Client{
		hc: &http.Client{
			Jar: jar,
		},
	}
}

func (c *Client) do(method, url string, params map[string]string, header map[string]string, body []byte) (*http.Response, error) {
	req, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	if params != nil {
		values := urlpkg.Values{}
		for k, v := range params {
			values.Set(k, v)
		}
		req.URL.RawQuery = values.Encode()
	}
	return c.hc.Do(req)
}

func (c *Client) Login(username, password string) error {
	// TODO:
	return nil
}

func (c *Client) Logout() error {
	jar, _ := cookiejar.New(nil)
	c.hc.Jar = jar
	return nil
}

func (c *Client) SetCookies(pubCookie string) error {
	u, err := urlpkg.Parse(apiEndpoint)
	if err != nil {
		return err
	}
	// TODO: verify cookie
	c.hc.Jar.SetCookies(u, []*http.Cookie{{Name: "pubcookie", Value: pubCookie}})
	c.isLogin = true
	return nil
}

func (c *Client) GetShareInfo(shareID, folderID string) (*Share, error) {
	url := fmt.Sprintf("%s%s", apiEndpoint, "/getdir.php")
	resp, err := c.do(http.MethodGet, url,
		map[string]string{"d": shareID, "folder_id": folderID},
		map[string]string{"Origin": origin}, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("StatusCode: %d", resp.StatusCode)
	}
	share := new(Share)
	if err := json.NewDecoder(utfbom.SkipOnly(resp.Body)).Decode(share); err != nil {
		return nil, err
	}
	if err := c.parseFiles(share); err != nil {
		return nil, err
	}
	return share, nil
}

func (c *Client) parseFiles(share *Share) error {
	url := fmt.Sprintf("%s%s", apiEndpoint, share.Url)
	resp, err := c.do(http.MethodGet, url, nil, map[string]string{"Origin": origin}, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("StatusCode: %d", resp.StatusCode)
	}
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	data := gjson.ParseBytes(b)
	data.Get("aaData").ForEach(func(_, value gjson.Result) bool {
		item := value.Array()
		file := &File{}
		file.Name = utils.Match1(`<a.*?>(.*?)</a>`, item[1].String())
		file.Size = item[2].String()
		file.Date = item[3].String()
		if utils.GetValueFromHTML(item[0].String(), "name") == "folder_ids[]" {
			file.Type = TypeFolder
		}
		switch file.Type {
		case TypeFile:
			file.ID = strings.Replace(utils.GetValueFromHTML(item[1].String(), "href"), "/file/", "", 1)
		case TypeFolder:
			file.ID = utils.GetValueFromHTML(item[0].String(), "value")
		}
		share.Files = append(share.Files, file)
		return true
	})
	return nil
}

func (c *Client) GetDownloadUrl(file *File) (map[string]string, error) {
	if file.Type != TypeFile {
		return nil, errors.New("this is not a file")
	}
	if !c.isLogin {
		return nil, errors.New("not login")
	}
	url := fmt.Sprintf("%s%s", apiEndpoint, "/getfile.php")
	resp, err := c.do(http.MethodGet, url,
		map[string]string{"f": file.ID},
		map[string]string{"Origin": origin,}, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("StatusCode: %d", resp.StatusCode)
	}
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	reg := regexp.MustCompile(`vip_(\D*)_url`)
	res := make(map[string]string)
	gjson.ParseBytes(b).ForEach(func(key, value gjson.Result) bool {
		match := reg.FindStringSubmatch(key.String())
		if match == nil || len(match) < 2 {
			return true
		}
		res[match[1]] = value.String()
		return true
	})
	return res, nil
}