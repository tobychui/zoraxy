/* Things to do before body loads */
function restoreDarkMode(){
    if (localStorage.getItem("darkMode") === "enabled") {
        $("html").addClass("is-dark");
        $("html").removeClass("is-white");
    } else {
        $("html").removeClass("is-dark");
        $("html").addClass("is-white");
    }
}
restoreDarkMode();

function updateElementToTheme(isDarkTheme=false){
    if (!isDarkTheme){
        let whiteSrc = $("#sysicon").attr("white_src");
        $("#sysicon").attr("src", whiteSrc);
        $("#darkModeToggle").html(`<span class="ts-icon is-sun-icon"></span>`);

        // Update the rendering text color in the garphs
        if (typeof(changeScaleTextColor) != "undefined"){
            changeScaleTextColor("black");
        }
    
    }else{
        let darkSrc = $("#sysicon").attr("dark_src");
        $("#sysicon").attr("src", darkSrc);
        $("#darkModeToggle").html(`<span class="ts-icon is-moon-icon"></span>`);
        
        // Update the rendering text color in the garphs
        if (typeof(changeScaleTextColor) != "undefined"){
            changeScaleTextColor("white");
        }
    }
}

/* Things to do after body loads */
$(document).ready(function(){
    $("#darkModeToggle").on("click", function() {
        $("html").toggleClass("is-dark");
        $("html").toggleClass("is-white");
        if ($("html").hasClass("is-dark")) {
            localStorage.setItem("darkMode", "enabled");
            updateElementToTheme(true);
        } else {
            localStorage.setItem("darkMode", "disabled");
            updateElementToTheme(false);
        }
    });

    updateElementToTheme(localStorage.getItem("darkMode") === "enabled");
});