fetch('http://localhost:8080/post-test', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json', },
  body: JSON.stringify({  "data": "Yeah!",  "other":  43}),
}).then((res) => { if (res.status == 200) { /* woohoo! */ } })
