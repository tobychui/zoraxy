// This script is used to scrape the DNS providers from the https://go-acme.github.io/lego/dns/ website
// It will fetch the DNS providers and their respective codes and store them in a Map object
// You can copy the code into the browser console and run it to get the Map object
// Dont forget to remove providers that are not supported by the current acme-lego version that is being used

const providerArray = [];
document.querySelectorAll('table a[href^="/lego/dns/"]').forEach((provider) => {
    fetch(provider.href)
        .then(function(response) { return response.text() })
        .then(function(html) {
            const parser = new DOMParser();
            const doc = parser.parseFromString(html, "text/html")

            const providerCodes = Array.from(doc.querySelector('table tbody').querySelectorAll('code')).map(code => code.innerHTML);
            const providerId = provider.href.match(/.*?\/dns\/(.*?)\//)[1];
            const providerName = provider.innerHTML;
            providerArray.push({providerId, providerName, providerCodes});
        })
        .catch(function(err) {  
            console.log('Failed to fetch page '+provider.href+': ', err);  
        });
})

// After fetching all the providers, sort them by providerName. You have to run this line in the console after the fetch is done

providerArray.sort((a,b) => a.providerName.localeCompare(b.providerName))


// Create Dropdown items for the providers

providerDropdownItems = "";
providerArray.forEach(provider => {
    providerDropdownItems += '<div class="item" data-value="'+provider.providerId+'">'+provider.providerName+'</div>\n'
})
console.log(providerDropdownItems);


// Create Credential prefill for the providers

switchCasePrefill = "";
providerArray.forEach(provider => {
    providerCodes = provider.providerCodes.reduce((accumulator,value) => accumulator + value + "=\\n","").slice(0, -2);
    switchCasePrefill += 'case "'+provider.providerId+'":\n\t$("#dnsCredentials").val("'+providerCodes+'");\n\tbreak;\n'
})
console.log(switchCasePrefill);