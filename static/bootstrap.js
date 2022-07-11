// Dispatch mandatory calls early.
var optionsPromise = fetch('/options');
var userInfoPromise = fetch('/user/info');

function render() {
  Promise.all([optionsPromise, userInfoPromise]).then((responses) => {
    return Promise.all(
      // TODO: Check that the response is fine (ok and status on the Response).
      responses.map((response) => response.json())
    );
  }).then((jsons) => {
    const option = jsons[0];
    const userInfo = jsons[1];
    const loggedIn = !!userInfo.access_token;
    const cash_available = userInfo.user_info.cash_available || null;

    var template = document.getElementById('template').innerHTML;
    var rendered = Mustache.render(template, { loggedIn: loggedIn, availableFortrading: cash_available, options: option.options, suggestions: option.suggestions });
    document.getElementById('target').innerHTML = rendered;
  })
  .catch((error) => {
    console.log('Failed loading options:' + error);
    document.getElementById('target').innerHTML = "Error loading options... Try reloading. If it happens again, let us know!";
  });
}

window.addEventListener('load', render);
