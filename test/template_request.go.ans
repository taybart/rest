req, err := http.NewRequest("POST", "http://localhost:8080/post-test", strings.NewReader(`{"data":"Yeah!","other":43}`))
req.Header.Set("Content-Type", "application/json")

res, err := http.DefaultClient.Do(req)
if err != nil {
  fmt.Println(err)
}
defer res.Body.Close()
body, err := ioutil.ReadAll(res.Body)
if err != nil {
  fmt.Println(err)
}
fmt.Println(string(body))
