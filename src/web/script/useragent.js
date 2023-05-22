/*
    User-Agent.js

    A utilities script that help render
    user agents by giving its raw UA header

    CopyRight tobychui. All Right Reserved
*/

 //parseUserAgent return the OS and browser name of the given ua
 function parseUserAgent(userAgent) {
    // browser
    var nVer = userAgent;
    var nAgt = userAgent;
    var browser = "";
    var version = '';
    var majorVersion = 0;
    var nameOffset, verOffset, ix;

    // Opera
    if ((verOffset = nAgt.indexOf('Opera')) != -1) {
        browser = 'Opera';
        version = nAgt.substring(verOffset + 6);
        if ((verOffset = nAgt.indexOf('Version')) != -1) {
            version = nAgt.substring(verOffset + 8);
        }
    }
    // Opera Next
    if ((verOffset = nAgt.indexOf('OPR')) != -1) {
        browser = 'Opera';
        version = nAgt.substring(verOffset + 4);
    }
    // Legacy Edge
    else if ((verOffset = nAgt.indexOf('Edge')) != -1) {
        browser = 'Microsoft Legacy Edge';
        version = nAgt.substring(verOffset + 5);
    } 
    // Edge (Chromium)
    else if ((verOffset = nAgt.indexOf('Edg')) != -1) {
        browser = 'Microsoft Edge';
        version = nAgt.substring(verOffset + 4);
    }
    // MSIE
    else if ((verOffset = nAgt.indexOf('MSIE')) != -1) {
        browser = 'Microsoft Internet Explorer';
        version = nAgt.substring(verOffset + 5);
    }
    // Chrome
    else if ((verOffset = nAgt.indexOf('Chrome')) != -1) {
        browser = 'Chrome';
        version = nAgt.substring(verOffset + 7);
    }
    // Safari
    else if ((verOffset = nAgt.indexOf('Safari')) != -1) {
        browser = 'Safari';
        version = nAgt.substring(verOffset + 7);
        if ((verOffset = nAgt.indexOf('Version')) != -1) {
            version = nAgt.substring(verOffset + 8);
        }
    }
    // Firefox
    else if ((verOffset = nAgt.indexOf('Firefox')) != -1) {
        browser = 'Firefox';
        version = nAgt.substring(verOffset + 8);
    }
    // MSIE 11+
    else if (nAgt.indexOf('Trident/') != -1) {
        browser = 'Microsoft Internet Explorer';
        version = nAgt.substring(nAgt.indexOf('rv:') + 3);
    }
    // Other browsers
    else if ((nameOffset = nAgt.lastIndexOf(' ') + 1) < (verOffset = nAgt.lastIndexOf('/'))) {
        browser = nAgt.substring(nameOffset, verOffset);
        version = nAgt.substring(verOffset + 1);
        if (browser.toLowerCase() == browser.toUpperCase()) {
            browser = navigator.appName;
        }
    }
    // trim the version string
    if ((ix = version.indexOf(';')) != -1) version = version.substring(0, ix);
    if ((ix = version.indexOf(' ')) != -1) version = version.substring(0, ix);
    if ((ix = version.indexOf(')')) != -1) version = version.substring(0, ix);

    majorVersion = parseInt('' + version, 10);
    if (isNaN(majorVersion)) {
        version = '' + parseFloat(navigator.appVersion);
        majorVersion = parseInt(navigator.appVersion, 10);
    }

    // mobile version
    var mobile = /Mobile|mini|Fennec|Android|iP(ad|od|hone)/.test(nVer);

    // system
    var os = "";
    var clientStrings = [
        {s:'Windows 10', r:/(Windows 10.0|Windows NT 10.0)/},
        {s:'Windows 8.1', r:/(Windows 8.1|Windows NT 6.3)/},
        {s:'Windows 8', r:/(Windows 8|Windows NT 6.2)/},
        {s:'Windows 7', r:/(Windows 7|Windows NT 6.1)/},
        {s:'Windows Vista', r:/Windows NT 6.0/},
        {s:'Windows Server 2003', r:/Windows NT 5.2/},
        {s:'Windows XP', r:/(Windows NT 5.1|Windows XP)/},
        {s:'Windows 2000', r:/(Windows NT 5.0|Windows 2000)/},
        {s:'Windows ME', r:/(Win 9x 4.90|Windows ME)/},
        {s:'Windows 98', r:/(Windows 98|Win98)/},
        {s:'Windows 95', r:/(Windows 95|Win95|Windows_95)/},
        {s:'Windows NT 4.0', r:/(Windows NT 4.0|WinNT4.0|WinNT|Windows NT)/},
        {s:'Windows CE', r:/Windows CE/},
        {s:'Windows 3.11', r:/Win16/},
        {s:'Android', r:/Android/},
        {s:'Open BSD', r:/OpenBSD/},
        {s:'Sun OS', r:/SunOS/},
        {s:'Chrome OS', r:/CrOS/},
        {s:'Linux', r:/(Linux|X11(?!.*CrOS))/},
        {s:'iOS', r:/(iPhone|iPad|iPod)/},
        {s:'Mac OS X', r:/Mac OS X/},
        {s:'Mac OS', r:/(Mac OS|MacPPC|MacIntel|Mac_PowerPC|Macintosh)/},
        {s:'QNX', r:/QNX/},
        {s:'UNIX', r:/UNIX/},
        {s:'BeOS', r:/BeOS/},
        {s:'OS/2', r:/OS\/2/},
        //Special agents
        {s:'Search Bot', r:/(nuhk|CensysInspect|facebookexternalhit|Twitterbot|AhrefsBot|Palo Alto|InternetMeasurement|PetalBot|coccocbot|MJ12bot|Googlebot|bingbot|Yammybot|YandexBot|SeznamBot|SemrushBot|Openbot|Slurp|Sogou web spider|MSNBot|Ask Jeeves\/Teoma|ia_archiver)/},
        {s: 'Scripts', r:/(Go-http-client|cpp-httplib|python-requests|Java|zgrab|ALittle Client|)/},
    ];
    for (var id in clientStrings) {
        var cs = clientStrings[id];
        if (cs.r.test(nAgt)) {
            os = cs.s;
            break;
        }
    }

    var osVersion = "";

    if (/Windows/.test(os)) {
        osVersion = /Windows (.*)/.exec(os)[1];
        os = 'Windows';
    }

    switch (os) {
        case 'Mac OS':
        case 'Mac OS X':
        case 'Android':
            osVersion = /(?:Android|Mac OS|Mac OS X|MacPPC|MacIntel|Mac_PowerPC|Macintosh) ([\.\_\d]+)/.exec(nAgt);
            if (osVersion != null){
                osVersion = osVersion[1];
            }else{
                osVersion = "";
            }
            break;

        case 'iOS':
            osVersion = /OS (\d+)_(\d+)_?(\d+)?/.exec(nVer);
            if (osVersion != null){
                osVersion = osVersion[1] + '.' + osVersion[2] + '.' + (osVersion[3] | 0);
            }else{
                osVersion = "";
            }
            
            break;
    }

    // Return OS and browser
    return {
        os: os,
        browser: browser,
        version: osVersion,
        isMobile: mobile,
    };
}

//Get OS color code give a persistant color
//if a OS or browser agent is known
//otherwise return light grey
function getOSColorCode(browserName){
    let browserColors = {
        'Windows 10': '#0078D7',
        'Windows 8.1': '#2D7D9A',
        'Windows 8': '#0063B1',
        'Windows 7': '#4C4C4C',
        'Windows Vista': '#008080',
        'Windows Server 2003': '#A30000',
        'Windows XP': '#6D6D6D',
        'Windows 2000': '#7F7F7F',
        'Windows ME': '#BDBDBD',
        'Windows 98': '#ECECEC',
        'Windows 95': '#F0F0F0',
        'Windows NT 4.0': '#9E9E9E',
        'Windows CE': '#FFDDBB',
        'Windows 3.11': '#BF2F2F',
        'Windows': '#6ec3f5',
        'Android': '#A4C639',
        'Open BSD': '#F5DEB3',
        'Sun OS': '#DAA520',
        'Chrome OS': '#4285F4',
        'Linux': '#ECECEC',
        'iOS': '#000000',
        'Mac OS X': '#A5A5A5',
        'Mac OS': '#A5A5A5',
        'QNX': '#696969',
        'UNIX': '#D3D3D3',
        'BeOS': '#8F8F8F',
        'OS/2': '#D8BFD8',
        'Search Bot': '#F6A821',
        'Scripts': '#B8B8B8',
    }

    return browserColors[browserName] || "#e0e0e0";
}