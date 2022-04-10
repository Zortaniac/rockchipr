# rockchipr

rockchipr is a tool to flash rk image files to RockChip devices in Maskrom mode. 
The functionality is similar to the [rkdevtool](https://github.com/rockchip-linux/rkdeveloptool/). 
The main difference is that it  also supports to rewrite IDB sector 3, which allows rewriting of:
* serial number
* IMEI
* UID
* network MAC
* bluetooth MAC


**This software comes with absolutely no warranty, use it on you own risk!** 

This software is a port of a dotnet application that was used to upgrade a few hundred thousand RK3128 based tablets. 
The purpose of the port was to learn go programming in a fun way. It was not tested with many and or any other RockChip based devices!   

Parts of this software are inspired from the [rkdevtool](https://github.com/rockchip-linux/rkdeveloptool/).