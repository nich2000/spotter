use framework "Foundation"
use scripting additions

on jsonEscape(value)
	set textValue to value as text
	set textValue to my replaceText(textValue, "\\", "\\\\")
	set textValue to my replaceText(textValue, "\"", "\\\"")
	set textValue to my replaceText(textValue, return, " ")
	set textValue to my replaceText(textValue, linefeed, " ")
	set textValue to my replaceText(textValue, tab, " ")
	return textValue
end jsonEscape

on replaceText(value, findText, replacementText)
	set AppleScript's text item delimiters to findText
	set parts to text items of value
	set AppleScript's text item delimiters to replacementText
	set joined to parts as text
	set AppleScript's text item delimiters to ""
	return joined
end replaceText

on isoDate(value)
	set totalSeconds to time of value
	set hourValue to totalSeconds div 3600
	set minuteValue to (totalSeconds mod 3600) div 60
	set secondValue to totalSeconds mod 60
	set timezoneValue to "+03:00"
	return (year of value as text) & "-" & my twoDigits(month of value as integer) & "-" & my twoDigits(day of value) & "T" & my twoDigits(hourValue) & ":" & my twoDigits(minuteValue) & ":" & my twoDigits(secondValue) & timezoneValue
end isoDate

on twoDigits(value)
	set numberValue to value as integer
	if numberValue < 10 then return "0" & numberValue
	return numberValue as text
end twoDigits

on run argv
	set messageLimit to 20
	if (count of argv) > 0 then set messageLimit to (item 1 of argv as integer)
	set rows to {}
	
	tell application "Mail"
		set inboxMessages to messages of inbox
		set totalMessages to count of inboxMessages
		set stopAt to messageLimit
		if totalMessages < stopAt then set stopAt to totalMessages
		repeat with i from 1 to stopAt
			set msg to item i of inboxMessages
			set msgSubject to subject of msg
			set msgSender to sender of msg
			set msgDate to date received of msg
			set msgUnread to ((read status of msg) is false)
			set mailboxName to "Inbox"
			try
				set mailboxName to name of mailbox of msg
			end try
			set row to "{\"subject\":\"" & my jsonEscape(msgSubject) & "\",\"sender\":\"" & my jsonEscape(msgSender) & "\",\"date\":\"" & my isoDate(msgDate) & "\",\"preview\":\"\",\"mailbox\":\"" & my jsonEscape(mailboxName) & "\",\"isUnread\":" & my boolJSON(msgUnread) & "}"
			set end of rows to row
		end repeat
	end tell
	
	set AppleScript's text item delimiters to ","
	set output to "[" & (rows as text) & "]"
	set AppleScript's text item delimiters to ""
	return output
end run

on boolJSON(value)
	if value then return "true"
	return "false"
end boolJSON
