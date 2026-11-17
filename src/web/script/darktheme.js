/*
    Dark Theme Toggle Manager

    This script is used to manage the dark theme toggle button in the header of the website.
    It will change the theme of the website to dark mode when the toggle is clicked and back to light mode when clicked again.

    Must be included just after the start of body tag in the HTML file.
*/

function _whiteThemeHandleApplyChange(){
    $(".menubar .logo").attr("src", "img/logo.svg");
}

function _darkThemeHandleApplyChange(){
    $(".menubar .logo").attr("src", "img/logo_white.svg");
}


 //Check if the theme is dark, must be done before the body is loaded to prevent flickering
 function setDarkTheme(isDarkTheme = false){
    if (isDarkTheme){
        $("body").addClass("darkTheme");
        $("#themeColorButton").html(`<i class="ui sun icon"></i>`);

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
        $("#themeColorButton").html(`<i class="ui moon icon"></i>`);
        //By default the page is white theme. So no need to change the logo if page is still loading
        if (document.readyState == "complete"){
            //Switching back to light theme
            _whiteThemeHandleApplyChange();
        }
    }
}

/*
    Theme selection:
    - If the user has clicked the toggle button, the choice is stored in localStorage
      ("dark" or "light", written by toggleTheme() in index.html) and always wins.
    - Otherwise, follow the system / browser setting (prefers-color-scheme),
      including live changes while the page is open.
*/
const _systemDarkQuery = window.matchMedia("(prefers-color-scheme: dark)");

function _hasStoredTheme(){
    let stored = localStorage.getItem("theme");
    return stored == "dark" || stored == "light";
}

if (_hasStoredTheme()){
    setDarkTheme(localStorage.getItem("theme") == "dark");
}else{
    setDarkTheme(_systemDarkQuery.matches);
}

_systemDarkQuery.addEventListener("change", function(e){
    if (!_hasStoredTheme()){
        setDarkTheme(e.matches);
    }
});