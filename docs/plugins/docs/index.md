# Index  

Welcome to the Zoraxy Plugin Documentation!  
Click on a topic in the side menu to begin navigating through the available resources and guides for developing and managing plugins.

## FAQ
### What skills do I need for developing a plugin?  
Basic HTML, JavaScript, and CSS skills are required, with Go (Golang) being the preferred backend language. However, any programming language that can be compiled into a binary and provide a web server interface will work.  

### Will a plugin crash the whole Zoraxy?  
No. Plugins operate in a separate process from Zoraxy. If a plugin crashes, Zoraxy will terminate and disable that plugin without affecting the core operations. This is by design to ensure stability.  

### Can I sell my plugin?  
Yes, the plugin library and interface design are open source under the LGPL license. You are not required to disclose the source code of your plugin as long as you do not modify the plugin library and use it as-is. For more details on how to comply with the license, refer to the licensing documentation.  

### How can I add my plugin to the official plugin store?  
To add your plugin to the official plugin store, open a pull request (PR) in the plugin repository.  