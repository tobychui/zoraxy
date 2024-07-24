/*
    Utils.js

    This script holds utilities function that is commonly use
    across components

*/

function abbreviateNumber(value) {
    var newValue = value;
    var suffixes = ["", "k", "m", "b", "t"];
    var suffixNum = 0;
    while (newValue >= 1000 && suffixNum < suffixes.length - 1) {
        newValue /= 1000;
        suffixNum++;
    }
    if (value > 1000){
        newValue = newValue.toFixed(2);
    }
    
    return newValue + suffixes[suffixNum];
}

Object.defineProperty(String.prototype, 'capitalize', {
    value: function() {
        return this.charAt(0).toUpperCase() + this.slice(1);
    },
    enumerable: false
});

//Add a new function to jquery for ajax override with csrf token injected
$.cjax = function(payload){
    let requireTokenMethod = ["POST", "PUT", "DELETE"];;
    if (requireTokenMethod.includes(payload.method) || requireTokenMethod.includes(payload.type)){
        //csrf token is required
        let csrfToken = document.getElementsByTagName("meta")["zoraxy.csrf.Token"].getAttribute("content");
        payload.headers = {
            "X-CSRF-Token": csrfToken,
        }
    }

    $.ajax(payload);
}