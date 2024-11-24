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
        localStorage.setItem("theme", "dark");

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
        localStorage.setItem("theme", "light");
        //By default the page is white theme. So no need to change the logo if page is still loading
        if (document.readyState == "complete"){
            //Switching back to light theme
            _whiteThemeHandleApplyChange();
        }
    }
}

if (localStorage.getItem("theme") == "dark"){
    setDarkTheme(true);
}else{
    setDarkTheme(false);
}