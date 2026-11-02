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
            title: "📁 Enable WebDAV Server",
            desc: `To manage files in your web directory remotely, enable the built-in <b>WebDAV server</b> by toggling <b>"Enable WebDAV Server"</b>. You can also customise the listen port and set custom credentials in the Advanced Options below.
            <br><br>The WebDAV server only listens on <code>localhost</code>, so you will need to expose it via a proxy rule in the next step.`,
            tab: "webserv",
            element: "#webserv .inline.field",
            scrollto: "#webserv .inline.field",
            pos: "bottomright",
        }),
        tourStepFactory({
            title: "🌐 Add DNS Record for WebDAV",
            desc: `Before creating the proxy rule, add a DNS <b>A</b> or <b>CNAME</b> record for your chosen WebDAV subdomain (e.g. <code>webdav.example.com</code>) pointing to your Zoraxy server's public IP address — just like you did for your root domain.
            <br><br>Once the DNS record is in place, fill in the subdomain in the <b>Matching Keyword / Domain</b> field.`,
            tab: "rules",
            element: "#rules .field[tourstep='matchingkeyword']",
            scrollto: "#rules .field[tourstep='matchingkeyword']",
            pos: "bottomright",
            callback: function(){
                let webdavPort = $("#webdav_port").val() || "5488";
                $("#rules .field[tourstep='matchingkeyword'] input").val("webdav.example.com");
                $("#rules .field[tourstep='targetdomain'] input").val("127.0.0.1:" + webdavPort);
            }
        }),
        tourStepFactory({
            title: "💾 Save the WebDAV Proxy Rule",
            desc: `The target destination has been pre-filled to <code>127.0.0.1:5488</code> (adjust if you changed the WebDAV port). Click <b>"Create Endpoint"</b> to save the rule.
            <br><br>Once saved, you can connect to <code>https://webdav.example.com</code> using any WebDAV-compatible client such as WinSCP, Cyberduck, or Windows "Map Network Drive".`,
            element: "#rules div[tourstep='newProxyRule']",
            scrollto: "#rules div[tourstep='newProxyRule']",
            pos: "topright",
        }),
        tourStepFactory({
            title: "🎉 WebDAV Server Setup Completed!",
            desc: `You should now be able to visit your domain and see the static web server contents show up in your browser.`,
            tab: "webserv",
            element: "",
            pos: "center",
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
            tab: "globalsettings",
            element: "#globalsettings div[tourstep='incomingPort']",
            scrollto: "#globalsettings div[tourstep='incomingPort']",
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
            element: "#globalsettings div[tourstep='forceHttpsRedirect']",
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
            desc: `If you didn't want to pay for a certificate, there are free CAs you can use to obtain a certificate. By default, Let's Encrypt is used and in order to use their service, you will need to fill in your contact email in the <b>ACME Email</b> field.
            <br><br>After filling in the email and selecting your preferred CA, click <b>Save</b> and continue.`,
            tab: "acme",
            element: "#acme div[tourstep='acmeSettings']",
            scrollto: "#acme div[tourstep='acmeSettings']",
            pos: "bottomright",
            callback: function(){
                openTabById("acme");
            }
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
            desc:`Free certificates only last for a few months. If you want Zoraxy to automate the renewal process, enable <b>"Enable Certificate Auto Renew"</b> using the toggle switch.
            <br><br>You can fine-tune which certificates get renewed in the <b>Advanced Renew Policy</b> section just below — choose "Renew All Certs" or select individual certificates from the table.`,
            tab: "acme",
            element: "#acmeAutoRenewCheckbox",
            scrollto: "#acmeAutoRenewCheckbox",
            pos: "bottomright",
            callback: function(){
                //Close the ACME side tool if it is still open from the previous step
                if ($(".sideWrapper").is(":visible")){
                    hideSideWrapperInTourMode();
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

    // Redirection rules tour steps
    "redirect": [
        tourStepFactory({
            title: "↩️ Redirection Rules",
            desc: `Sometimes you want to send visitors from one URL straight to another — for example, redirecting an old domain to a new one, or sending <code>http://</code> traffic to <code>https://</code>. That's exactly what <b>Redirection Rules</b> are for.
            <br><br>In this tour you'll learn how to create a redirection rule and tune it to your needs.`,
            tab: "redirectset",
            pos: "center",
        }),
        tourStepFactory({
            title: "📋 Existing Rules",
            desc: `This table lists all your current redirection rules. Each row shows the source URL, the destination URL, and whether pathname forwarding and device filtering are active.
            <br><br>You can toggle rules on/off with the switch in the first column, or delete them with the trash icon.`,
            tab: "redirectset",
            element: "#redirectset table",
            scrollto: "#redirectset table",
            pos: "bottomright",
        }),
        tourStepFactory({
            title: "🌐 Redirection URL (From)",
            desc: `Fill in the URL you want to redirect <b>from</b> — this is the address your visitors currently use.
            <br><br>e.g. <code>old.example.com</code>. Any incoming request whose URL starts with this value will be caught by this rule.`,
            element: "#redirectset [tourstep='url-from']",
            scrollto: "#redirectset [tourstep='url-from']",
            pos: "bottomright",
        }),
        tourStepFactory({
            title: "🎯 Destination URL (To)",
            desc: `Fill in the URL you want to redirect visitors <b>to</b>.
            <br><br>e.g. <code>new.example.com</code> or <code>new.example.com/landing/</code>. You can include a path segment here — a trailing slash is sometimes required depending on how the destination server handles requests.`,
            element: "#redirectset [tourstep='url-to']",
            scrollto: "#redirectset [tourstep='url-to']",
            pos: "bottomright",
        }),
        tourStepFactory({
            title: "📂 Forward Pathname",
            desc: `The <b>Forward Pathname</b> option controls whether the path after the source URL is appended to the destination.
            <br><br>
            <b>Enabled:</b> <code>old.example.com<b>/blog?post=13</b></code> → <code>new.example.com<b>/blog?post=13</b></code><br>
            <b>Disabled:</b> <code>old.example.com/blog?post=13</code> → <code>new.example.com</code>
            <br><br>Leave it <b>checked</b> (default) if you want all sub-paths to follow along with the redirect.`,
            element: "#redirectset .field:has(input[name='forward-childpath'])",
            scrollto: "#redirectset .field:has(input[name='forward-childpath'])",
            pos: "bottomright",
        }),
        tourStepFactory({
            title: "📱 Device Type Filter",
            desc: `You can optionally restrict a redirection rule to only fire for certain device types.
            <br><br>
            <b>All Devices</b> — rule applies to everyone (default).<br>
            <b>Desktop Only</b> — only desktop browsers are redirected; mobile visitors continue normally.<br>
            <b>Mobile Only</b> — only mobile browsers are redirected; useful for sending mobile users to a dedicated mobile site.
            <br><br>Unknown device types are treated as desktop.`,
            element: "#redirectset .grouped.fields:has(input[name='redirect-device'])",
            scrollto: "#redirectset .grouped.fields:has(input[name='redirect-device'])",
            pos: "bottomright",
        }),
        tourStepFactory({
            title: "🔖 Status Code",
            desc: `Choose between a <b>307 Temporary Redirect</b> and a <b>301 Moved Permanently</b>.
            <br><br>
            Use <b>307</b> while testing — browsers won't cache it, so you can change the rule freely.<br>
            Switch to <b>301</b> once you're confident the redirect is permanent; search engines will transfer SEO ranking to the new URL.`,
            element: "#redirectset .grouped.fields:has(input[name='redirect-type'])",
            scrollto: "#redirectset .grouped.fields:has(input[name='redirect-type'])",
            pos: "topright",
        }),
        tourStepFactory({
            title: "💾 Save the Redirection Rule",
            desc: `Happy with the settings? Click <b>"Add Redirection Rule"</b> to activate it immediately — no restart required.
            <br><br>The new rule will appear at the top of the rules table. You can come back and edit or delete it at any time.`,
            element: "#redirectset [tourstep='save_redirect_btn']",
            scrollto: "#redirectset [tourstep='save_redirect_btn']",
            pos: "topright",
        }),
        tourStepFactory({
            title: "🎉 Redirection Rule Added!",
            desc: `The new redirection rule has been successfully added and is now active.`,
            element: "",
            scrollto: "#redirectset",
            pos: "center",
        }),
    ],

    // Stream proxy (TCP/UDP) tour steps
    "stream": [
        tourStepFactory({
            title: "🌊 Stream Proxy (TCP / UDP)",
            desc: `The <b>Stream Proxy</b> lets you forward raw TCP or UDP traffic to a backend server — think game servers, databases, VoIP, or anything that doesn't speak HTTP.
            <br><br>In this tour we'll use a <b>Minecraft server</b> as a hands-on example.`,
            tab: "streamproxy",
            pos: "center",
        }),
        tourStepFactory({
            title: "🤔 How is this different from HTTP Proxy?",
            desc: `HTTP proxy rules can tell traffic apart by domain name, so many sites can share the same port 80/443. Stream proxy doesn't work that way — it just forwards everything arriving on a port straight to one backend, no questions asked.
            <br><br>That's a perfect fit for game servers, VoIP, and other non-HTTP services. If you do need smarter per-connection routing, check out the <b>Proxy Protocol</b> option at the end of this tour.`,
            tab: "streamproxy",
            pos: "center",
        }),
        tourStepFactory({
            title: "📋 Existing Stream Rules",
            desc: `This table lists all active stream proxy rules. Each row shows the rule name, the port Zoraxy is listening on, and the backend address traffic is forwarded to.`,
            element: "#streamproxy #proxyTable",
            scrollto: "#streamproxy #proxyTable",
            pos: "bottomright",
        }),
        tourStepFactory({
            title: "🏷️ Give It a Name",
            desc: `Fill in a friendly name for this rule so you can identify it later.
            <br><br>For our example, type something like <code>Minecraft Server</code>.`,
            element: "#streamproxy #streamProxyForm .field:has(input[name='name'])",
            scrollto: "#streamproxy #addproxyConfig",
            pos: "bottomright",
        }),
        tourStepFactory({
            title: "👂 Listening Address",
            desc: `This is the port on <em>this</em> machine that Zoraxy will listen on for incoming connections.
            <br><br>Minecraft's default port is <code>25565</code>, so enter <code>:25565</code> to listen on all interfaces, or <code>0.0.0.0:25565</code> explicitly.
            <br><br><b>Docker users:</b> you must also expose this port in your <code>docker-compose.yml</code> or <code>-p</code> flag.`,
            element: "#streamproxy #streamProxyForm .field:has(input[name='listenAddr'])",
            scrollto: "#streamproxy #streamProxyForm .field:has(input[name='listenAddr'])",
            pos: "bottomright",
        }),
        tourStepFactory({
            title: "🎯 Proxy Target Address",
            desc: `Enter the address of your actual Minecraft server backend — the machine Zoraxy should forward traffic to.
            <br><br>e.g. <code>192.168.1.100:25565</code> for a server on your local network, or <code>mc.myserver.com:25565</code> for a remote host.`,
            element: "#streamproxy #streamProxyForm .field:has(input[name='proxyAddr'])",
            scrollto: "#streamproxy #streamProxyForm .field:has(input[name='proxyAddr'])",
            pos: "bottomright",
        }),
        tourStepFactory({
            title: "⚙️ Enable TCP / UDP",
            desc: `Toggle on <b>TCP</b> for Minecraft Java Edition (it uses TCP). If you are also forwarding a Bedrock Edition server, enable <b>UDP</b> as well.
            <br><br>You can enable both on the same rule if your backend handles both protocols on the same port.`,
            element: "#streamproxy #streamProxyForm .field:has(input[name='useTCP'])",
            scrollto: "#streamproxy #streamProxyForm .field:has(input[name='useTCP'])",
            pos: "bottomright",
        }),
        tourStepFactory({
            title: "🔬 Proxy Protocol (Advanced)",
            desc: `Leave this set to <b>Disabled</b> for most use cases like Minecraft.
            <br><br>If your backend needs to know the <em>real client IP address</em> (e.g. a web server behind Zoraxy), enable <b>Proxy Protocol V1 or V2</b>. It prepends a small header to each connection so the backend can see where the traffic originally came from — think of it as writing the sender's return address on the envelope before forwarding it.
            <br><br>Note: UDP does not support Proxy Protocol V1.`,
            element: "#streamproxy #streamProxyForm .field:has(select[name='proxyProtocolVersion'])",
            scrollto: "#streamproxy #streamProxyForm .field:has(select[name='proxyProtocolVersion'])",
            pos: "topright",
        }),
        tourStepFactory({
            title: "💾 Create the Stream Rule",
            desc: `Click <b>"Create"</b> to start the stream proxy immediately. Your Minecraft server will now be accessible through Zoraxy on port <code>25565</code>.
            <br><br>Players can connect using your Zoraxy host's domain or IP address — Zoraxy will transparently forward all traffic to your backend server.`,
            element: "#streamproxy [tourstep='create_stream_proxy_btn']",
            scrollto: "#streamproxy [tourstep='create_stream_proxy_btn']",
            pos: "topright",
        }),
         tourStepFactory({
            title: "🎉Stream Proxy Rule Added!",
            desc: `The new stream proxy rule has been successfully added and is now active.`,
            element: "",
            scrollto: "#streamproxy",
            pos: "center",
        }),
    ],
}