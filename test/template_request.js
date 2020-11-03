fetch('http://localhost:8080/post-test', {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
  },
  body: JSON.stringify({"data":"Yeah!","other":43}),
})
  .then((res) => res.json().then((data) => ({ status: res.status, data })))
  .then(({ status, data }) => console.log(status, data))
