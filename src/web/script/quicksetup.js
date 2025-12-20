/* 
    Quick Setup Tour

    This script file contains all the required script
    for quick setup tour and walkthrough
*/

//tourStepFactory generate a function that renders the steps in tourModal
//Keys: {element, title, desc, tab, pos, scrollto, callback}
//      elements -> Element (selector) to focus on
//      tab -> Tab ID to switch pages
//      pos -> Where to display the tour modal, {topleft, topright, bottomleft, bottomright, center}
//      scrollto -> Element (selector) to scroll to, can be different from elements
//      ignoreVisiableCheck -> Force highlight even if element is currently not visable
function adjustTourModalOverlayToElement(element){;
    if ($(element) == undefined || $(element).offset() == undefined){
        return;
    }

    let padding = 12;
    $("#tourModalOverlay").css({
        "top": $(element).offset().top - padding - $(document).scrollTop(),
        "left": $(element).offset().left - padding,
        "width": $(element).width() + 2 * padding,
        "height": $(element).height() + 2 * padding,
    });
}

var tourOverlayUpdateTicker;

function tourStepFactory(config){
    return function(){
         //Check if this step require tab swap
         if (config.tab != undefined && config.tab != ""){
            //This tour require tab swap. call to openTabById
            openTabById(config.tab);
        }

        if (config.ignoreVisiableCheck == undefined){
            config.ignoreVisiableCheck = false;
        }
        
        if (config.element == undefined || (!$(config.element).is(":visible") && !config.ignoreVisiableCheck)){
            //No focused element in this step.
            $(".tourFocusObject").removeClass("tourFocusObject");
            $("#tourModal").addClass("nofocus");
            $("#tourModalOverlay").hide();

            //If there is a target element to scroll to
            if (config.scrollto != undefined){
                $('html, body').animate({
                    scrollTop: $(config.scrollto).offset().top - 100
                }, 500);
            }

        }else{

            let elementHighligher = function(){
                //Match the overlay to element position and size
                $(window).off("resize").on("resize", function(){
                    adjustTourModalOverlayToElement(config.element);
                });
                if (tourOverlayUpdateTicker != undefined){
                    clearInterval(tourOverlayUpdateTicker);
                }
                tourOverlayUpdateTicker = setInterval(function(){
                    adjustTourModalOverlayToElement(config.element);
                }, 500);
                adjustTourModalOverlayToElement(config.element);
                $("#tourModalOverlay").fadeIn();
            }

            //Consists of focus element in this step
            $(".tourFocusObject").removeClass("tourFocusObject");
            $(config.element).addClass("tourFocusObject");
            $("#tourModal").removeClass("nofocus");
            $("#tourModalOverlay").hide();
             //If there is a target element to scroll to
             if (config.scrollto != undefined){
                $('html, body').animate({
                    scrollTop: $(config.scrollto).offset().top - 100
                }, 300, function(){
                    setTimeout(elementHighligher, 300);
                });
            }else{
                setTimeout(elementHighligher, 300);
            }
        }

        //Get the modal location of this step
        let showupZone = "center";
        if (config.pos != undefined){
            showupZone = config.pos
        }

        $("#tourModal").attr("position", showupZone);

        $("#tourModal .tourStepTitle").html(config.title);
        $("#tourModal .tourStepContent").html(config.desc);
        if (config.callback != undefined){
            config.callback();
        }

       
    }
}

//Hide the side warpper in tour mode and prevent body from restoring to
//overflow scroll mode
function hideSideWrapperInTourMode(){
    hideSideWrapper(); //Call to index.html hide side wrapper function
    $("body").css("overflow", "hidden"); //Restore overflow state
}

function startQuickStartTour(){
    if (currentQuickSetupClass == ""){
        msgbox("No selected setup service tour", false);
        return;
    }   
    //Show the tour modal
    $("#tourModal").show();
    //Load the tour steps
    if (tourSteps[currentQuickSetupClass] == undefined || tourSteps[currentQuickSetupClass].length == 0){
        //This tour is not defined or empty
        let notFound = tourStepFactory({
            title: "😭 Tour not found",
            desc: "Seems you are requesting a tour that has not been developed yet. Check back on later!"
        });
        notFound();

        //Enable the finish button
        $("#tourModal .nextStepAvaible").hide();
        $("#tourModal .nextStepFinish").show();

        //Set step counter to 1
        $("#tourModal .tourStepCounter").text("0 / 0");
        return;
    }else{
        tourSteps[currentQuickSetupClass][0]();
    }
    
    updateTourStepCount();
    
    //Disable the previous button
    if (tourSteps[currentQuickSetupClass].length == 1){
        //There are only 1 step in this tour
        $("#tourModal .nextStepAvaible").hide();
        $("#tourModal .nextStepFinish").show();
    }else{
        $("#tourModal .nextStepAvaible").show();
        $("#tourModal .nextStepFinish").hide();
    }
    $("#tourModal .tourStepButtonBack").addClass("disabled");

    //Disable body scroll and let tour steps to handle scrolling
    $("body").css("overflow-y","hidden");
    $("#mainmenu").css("pointer-events", "none");
}

function updateTourStepCount(){
    let tourlistLength = tourSteps[currentQuickSetupClass]==undefined?1:tourSteps[currentQuickSetupClass].length;
    $("#tourModal .tourStepCounter").text((currentQuickSetupTourStep + 1) + " / " + tourlistLength);
}

function nextTourStep(){
    //Add one to the tour steps
    currentQuickSetupTourStep++;
    if (currentQuickSetupTourStep == tourSteps[currentQuickSetupClass].length - 1){
        //Already the last step
        $("#tourModal .nextStepAvaible").hide();
        $("#tourModal .nextStepFinish").show();
    }
    updateTourStepCount();
    tourSteps[currentQuickSetupClass][currentQuickSetupTourStep]();
    if (currentQuickSetupTourStep > 0){
        $("#tourModal .tourStepButtonBack").removeClass("disabled");
    }
}

function previousTourStep(){
    if (currentQuickSetupTourStep > 0){
        currentQuickSetupTourStep--;
    }

    if (currentQuickSetupTourStep != tourSteps[currentQuickSetupClass].length - 1){
        //Not at the last step
        $("#tourModal .nextStepAvaible").show();
        $("#tourModal .nextStepFinish").hide();
    }

    if (currentQuickSetupTourStep == 0){
        //Cant go back anymore
        $("#tourModal .tourStepButtonBack").addClass("disabled");
    }
    updateTourStepCount();
    tourSteps[currentQuickSetupClass][currentQuickSetupTourStep]();
}

//End tour and reset everything
function endTourFocus(){
    $(".tourFocusObject").removeClass("tourFocusObject");
    $(".serviceOption.active").removeClass("active");
    currentQuickSetupClass = "";
    currentQuickSetupTourStep = 0;
    $("#tourModal").hide();
    $("#tourModal .nextStepAvaible").show();
    $("#tourModal .nextStepFinish").hide();
    $("#tourModalOverlay").hide();
    if (tourOverlayUpdateTicker != undefined){
        clearInterval(tourOverlayUpdateTicker);
    }
    $("body").css("overflow-y","auto");
    $("#mainmenu").css("pointer-events", "auto");
}


var tourSteps = {
    //Homepage steps
    "homepage": [
        tourStepFactory({
            title: "🎉 Congratulation on your first site!",
            desc: "In this tour, you will be guided through the steps required to setup a basic static website using your own domain name with Zoraxy."
        }),
        tourStepFactory({
            title: "👉 Pointing domain DNS to Zoraxy's IP",
            desc: `Setup a DNS A Record that points your domain name to this Zoraxy instances public IP address. <br>
            Assume your public IP is 93.184.215.14, you should have an A record like this.
            <table class="ui celled collapsing basic striped table">
                <thead>
                    <tr>
                        <th>Name</th>
                        <th>Type</th>
                        <th>Value</th>
                    </tr>   
                </thead>
                <tbody>
                    <tr>
                        <td>yourdomain.com</td>
                        <td>A</td>
                        <td>93.184.215.14</td>
                    </tr>
                </tbody>
            </table>
            <br>If the IP of Zoraxy start from 192.168, you might want to use your router's public IP address and setup port forward for both port 80 and 443 as well.`,
            callback: function(){
                $.get("/api/acme/wizard?step=10", function(data){
                    if (data.error == undefined){
                        //Should return the public IP address from acme wizard
                        //Overwrite the sample IP address
                        let originalText = $("#tourModal .tourStepContent").html();
                        originalText = originalText.split("93.184.215.14").join(data);
                        $("#tourModal .tourStepContent").html(originalText);
                    }
                })
            }
        }),
        tourStepFactory({
            title: "🏠 Setup Default Site",
            desc: `If you already have an apache or nginx web server running, use "Reverse Proxy Target" and enter your current web server IP address. <br>Otherwise, pick "Internal Static Web Server" and click "Apply Change"`,
            tab: "setroot",
            element: "#setroot",
            pos: "bottomright"
        }),
        tourStepFactory({
            title: "🌐 Enable Static Web Server",
            desc: `Enable the static web server if it is not already enabled. Skip this step if you are using external web servers like Apache or Nginx.`,
            tab: "webserv",
            element: "#webserv",
            pos: "bottomright"
        }),
        tourStepFactory({
            title: "📤 Upload Static Website",
            desc: `Upload your static website files (e.g. HTML files) to the web directory. If remote access is not avaible, you can also upload it with the web server file manager here.`,
            tab: "webserv",
            element: "#webserv_dirManager",
            pos: "bottomright",
            scrollto: "#webserv_dirManager"
        }),
        tourStepFactory({
            title: "💡 Start Zoraxy HTTP listener",
            desc: `Start Zoraxy (if it is not already running) by pressing the "Start Service" button.<br>You should now be able to visit your domain and see the static web server contents show up in your browser.`,
            tab: "status",
            element: "#status .poweroptions",
            pos: "bottomright",
        })
    ],

    //Subdomains tour steps
    "subdomain":[
        tourStepFactory({
            title: "🎉 Creating your first subdomain",
            desc: "Seems you are now ready to expand your site with more services! To do so, you can create a new subdomain for your new web services. <br><br>In this tour, you will be guided through the steps to setup a new subdomain reverse proxy.",
            pos: "center"
        }),
        tourStepFactory({
            title: "👉 Pointing subdomain DNS to Zoraxy's IP",
            desc: `Setup a DNS CNAME Record that points your subdomain to your root domain. <br>
            Assume your public IP is 93.184.215.14, you should have an CNAME record like this.
            <table class="ui celled collapsing basic striped table">
                <thead>
                    <tr>
                        <th>Name</th>
                        <th>Type</th>
                        <th>Value</th>
                    </tr>   
                </thead>
                <tbody>
                    <tr>
                        <td>example.com</td>
                        <td>A</td>
                        <td>93.184.215.14</td>
                    </tr>
                    <tr>
                        <td>sub.example.com</td>
                        <td>CNAME</td>
                        <td>example.com</td>
                    </tr>
                </tbody>
            </table>`,
            callback: function(){
                $.get("/api/acme/wizard?step=10", function(data){
                    if (data.error == undefined){
                        //Should return the public IP address from acme wizard
                        //Overwrite the sample IP address
                        let originalText = $("#tourModal .tourStepContent").html();
                        originalText = originalText.split("93.184.215.14").join(data);
                        $("#tourModal .tourStepContent").html(originalText);
                    }
                })
            }
        }),
        tourStepFactory({
            title: "➕ Create New Proxy Rule",
            desc: `Next, you can now move on to create a proxy rule that reverse proxy your new subdomain in Zoraxy. You can easily add new rules using the "New Proxy Rule" web form.`,
            tab: "rules",
            pos: "topright"
        }),
        tourStepFactory({
            title: "🌐 Matching Keyword / Domain",
            desc: `Fill in your new subdomain in the "Matching Keyword / Domain" field.<br> e.g. sub.example.com`,
            element: "#rules .field[tourstep='matchingkeyword']",
            pos: "bottomright"
        }),
        tourStepFactory({
            title: "🖥️ Target IP Address or Domain Name with port",
            desc: `Fill in the Reverse Proxy Destination. e.g. localhost:8080 or 192.168.1.100:9096. <br><br>Please make sure your web services is accessible by Zoraxy.`,
            element: "#rules .field[tourstep='targetdomain']",
            pos: "bottomright"
        }),
        tourStepFactory({
            title: "🔐 Proxy Target require TLS Connection",
            desc: `If your upstream service only accept https connection, select this option.`,
            element: "#rules .field[tourstep='requireTLS']",
            pos: "bottomright",
           
        }),
        tourStepFactory({
            title: "🔓 Ignore TLS Validation Error",
            desc: `Some open source projects like Proxmox or NextCloud use self-signed certificate to serve its web UI. If you are proxying services like that, enable this option. `,
            element: "#rules #advanceProxyRules .field[tourstep='skipTLSValidation']",
            scrollto: "#rules #advanceProxyRules",
            pos: "bottomright",
            ignoreVisiableCheck: true,
            callback: function(){
                $("#advanceProxyRules").accordion();
                if (!$("#rules #advanceProxyRules .content").is(":visible")){
                    //Open up the advance config menu
                    $("#rules #advanceProxyRules .title")[0].click()
                }
            }
        }),
        tourStepFactory({
            title: "💾 Save New Proxy Rule",
            desc: `Now, click "Create Endpoint" to add this reverse proxy rule to runtime.`,
            element: "#rules div[tourstep='newProxyRule']",
            scrollto: "#rules div[tourstep='newProxyRule']",
            pos: "topright",
        }),
        tourStepFactory({
            title: "🎉 New Proxy Rule Setup Completed!",
            desc: `You can continue to add more subdomains or alias domain using this web form. To view the created reverse proxy rules, you can navigate to the HTTP Proxy tab.`,
            element: "#rules",
            tab: "rules",
            pos: "bottomright",
        }),
        tourStepFactory({
            title: "🌲 HTTP Proxy List",
            desc: `In this tab, you will see all the created HTTP proxy rules and edit them if needed. You should see your newly created HTTP proxy rule in the above list. <Br><Br>
                    This is the end of this tour. If you want further documentation on how to setup access control filters or load balancer, check out our Github Wiki page.`,
            element: "#httprp",
            tab: "httprp",
            pos: "bottomright",
        }),
    ],

    //TLS and ACME tour steps
    "tls":[
        tourStepFactory({
            title: "🔐 Enable HTTPS (TLS) for your site",
            desc: `Some technologies only work with HTTPS for security reasons. In this tour, you will be guided through the steps to enable HTTPS in Zoraxy.`,
            pos: "center",
        }),
        tourStepFactory({
            title: "➡️ Change Listening Port",
            desc: `HTTPS listen on port 443 instead of 80. If your Zoraxy is currently listening to ports other than 443, change it to 443 in incoming port option and click "Apply"`,
            tab: "status",
            element: "#status div[tourstep='incomingPort']",
            scrollto: "#status div[tourstep='incomingPort']",
            pos: "bottomright",
        }),
        tourStepFactory({
            title: "🔑 Enable TLS Serving",
            desc: `Next, you can enable TLS by checking the "Use TLS to serve proxy request"`,
            element: "#tls",
            scrollto: "#tls",
            pos: "bottomright",
        }),
        tourStepFactory({
            title: "💻 Enable HTTP Server on Port 80",
            desc: `As we might want some proxy rules to be accessible by HTTP, turn on the HTTP server listener on port 80 as well.`,
            element: "#listenP80",
            scrollto: "#tls",
            pos: "bottomright",
        }),
        tourStepFactory({
            title: "↩️ Force redirect HTTP request to HTTPS",
            desc: `By default, if a HTTP host-name is not found, 404 error page will be returned. However, in common scenerio for self-hosting, you might want to redirect that request to your HTTPS server instead. <br><br>Enabling this option allows such redirection to be done automatically.`,
            element: "#status div[tourstep='forceHttpsRedirect']",
            scrollto: "#tls",
            pos: "bottomright",
        }),
        tourStepFactory({
            title: "🎉 HTTPS Enabled!",
            desc: `Now, your Zoraxy instance is ready to serve HTTPS requests. 
            <br><br>By default, Zoraxy serve all your host-names by its internal self-signed certificate which is not a proper setup. That is why you will need to request a proper certificate for your site from your ISP or CA. `,
            tab: "status",
            pos: "center",
        }),
        tourStepFactory({
            title: "🔐 TLS / SSL Certificates",
            desc: `Zoraxy come with a simple and handy TLS management interface, where you can upload or request your certificates with a web form. You can click "TLS / SSL Certificate" from the side menu to open this page.`,
            tab: "cert",
            element: "#mainmenu",
            pos: "center",
        }),
        tourStepFactory({
            title: "⚙️ Setup ACME",
            desc: `If you didn't want to pay for a certificate, there are free CA where you can use to obtain a certificate. By default, Let's Encrypt is used and in order to use their service, you will need to fill in your webmin contact email in the "ACME EMAIL" field.
            <br><br> After you are done, click "Save Settings" and continue.`,
            element: "#cert div[tourstep='acmeSettings']",
            scrollto: "#cert div[tourstep='acmeSettings']",
            pos: "bottomright",
        }),
        tourStepFactory({
            title: "👉 Open ACME Tool",
            desc: `Open the ACME Tool by pressing the button below the ACME settings. You will see a tool window popup from the side.`,
            element: ".sideWrapper",
            pos: "center",
            callback: function(){
                //Call to function in cert.html
                openACMEManager();
            }
        }),
        tourStepFactory({
            title: "📃 Obtain Certificate with ACME",
            desc: `Now, we can finally start requesting a free certificate from the selected CA. Fill in the "Generate New Certificate" web-form and click <b>"Get Certificate"</b>.
            This usually will takes a few minutes. Wait until the spinning icon disappear before moving on the next step. 
            <br><br>Tips: You can check the "Use DNS Challenge" if you are trying to request a certificate containing wildcard character (*).`,
            element: ".sideWrapper",
            pos: "topleft",
        }),
        tourStepFactory({
            title: "🔄 Enable Auto Renew",
            desc:`Free certificate only last for a few months. If you want Zoraxy to help you automate the certificate renew process, enable "Auto Renew" by clicking the <b>"Enable Certificate Auto Renew"</b> toggle switch.
            <br><br>You can fine tune which certificate to renew in the "Advance Renew Policy" dropdown.`,
            element: ".sideWrapper",
            pos: "bottomleft",
            callback: function(){
                //If the user arrive this step from "Back"
                if (!$(".sideWrapper").is(":visible")){
                    openACMEManager();
                }
            }
        }),
        tourStepFactory({
            title: "🎉 Certificate Installed!",
            desc:`Now, your certificate is loaded into the database and it is ready to use! In Zoraxy, you do not need to manually assign the certificate to a domain. Zoraxy will do that automatically for you. 
                <br><br>Now, you can try to visit your website with https:// and see your green lock shows up next to your domain name!`,
            element: "#cert div[tourstep='certTable']",
            scrollto: "#cert div[tourstep='certTable']",
            pos: "bottomright",
            callback: function(){
                hideSideWrapperInTourMode();
            }
        }),

    ],
}