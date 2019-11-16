fetch('http://localhost:8080/post-test', {
  method: 'POST',
  headers: {
    "Content-Type": "application/json",

  },
  body: {  "data": "Yeah!"}
}).then((res) => {
  if (res.status == 200) {
    // woohoo!
  }
})
