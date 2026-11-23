/*
    Dark Theme Manager

    This script is used to manage the theme (light / dark) of the website.
    The user can choose between "light", "dark" and "system" with the theme selector in the header.

    - "light" and "dark" are an explicit choice, stored in localStorage["theme"]
    - "system" removes the stored value. The theme then follows the system / browser
      setting (prefers-color-scheme), including live changes while the page is open.

    Must be included just after the start of body tag in the HTML file.
*/

function _whiteThemeHandleApplyChange(){
    $(".menubar .logo").attr("src", "img/logo.svg");
}

function _darkThemeHandleApplyChange(){
    $(".menubar .logo").attr("src", "img/logo_white.svg");
}


 //Check if the theme is dark, must be done before the body is loaded to prevent flickering
 //This only applies the theme, it does not store the choice of the user
 function setDarkTheme(isDarkTheme = false){
    if (isDarkTheme){
        $("body").addClass("darkTheme");

        //Check if the page is still loading, if not change the logo
        if (document.readyState == "complete"){
            _darkThemeHandleApplyChange();
        }else{
            //Wait for the page to load and then change the logo
            $(document).ready(function(){
                _darkThemeHandleApplyChange();
            });
        }
    }else{
        $("body").removeClass("darkTheme")
        //By default the page is white theme. So no need to change the logo if page is still loading
        if (document.readyState == "complete"){
            //Switching back to light theme
            _whiteThemeHandleApplyChange();
        }
    }
}

const _systemDarkQuery = window.matchMedia("(prefers-color-scheme: dark)");

//Get the theme mode chosen by the user: "light", "dark" or "system" (nothing stored)
function getThemeMode(){
    let stored = localStorage.getItem("theme");
    if (stored == "dark" || stored == "light"){
        return stored;
    }
    return "system";
}

function _isDarkThemeMode(mode){
    if (mode == "system"){
        return _systemDarkQuery.matches;
    }
    return mode == "dark";
}

//Highlight the button of the current mode in the theme selector, if the page has one
function _updateThemeSelector(mode){
    $(".themeModeButton").removeClass("active");
    $(".themeModeButton[data-theme-mode='" + mode + "']").addClass("active");
}

//Apply the theme mode chosen by the user and remember the choice
function setThemeMode(mode){
    if (mode == "system"){
        localStorage.removeItem("theme");
    }else{
        localStorage.setItem("theme", mode);
    }
    setDarkTheme(_isDarkThemeMode(mode));
    _updateThemeSelector(mode);
}

setDarkTheme(_isDarkThemeMode(getThemeMode()));
$(document).ready(function(){
    _updateThemeSelector(getThemeMode());
});

//Follow live changes of the system theme, unless the user has chosen light or dark
_systemDarkQuery.addEventListener("change", function(e){
    if (getThemeMode() == "system"){
        setDarkTheme(e.matches);
    }
});
