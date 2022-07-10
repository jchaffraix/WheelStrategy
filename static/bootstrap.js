// Dispatch mandatory calls early.
optionsPromise = fetch('/options')

function render() {
  optionsPromise.then((response) => {
    // TODO: Check that the response is fine (ok and status on the Response).
    return response.json()
  }).then((json) => {
    var loggedIn = !!json.tda_auth;

    var template = document.getElementById('template').innerHTML;
    var rendered = Mustache.render(template, { loggedIn: loggedIn, options: json.options, suggestions: json.suggestions });
    document.getElementById('target').innerHTML = rendered;
  })
  .catch((error) => {
    console.log('Failed loading options:' + error);
    document.getElementById('target').innerHTML = "Error loading options... Try reloading. If it happens again, let us know!";
  });
}

window.addEventListener('load', render);
