use framework "Foundation"
use scripting additions

property rows : {}
property seenIDs : {}

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
	set seenIDs to {}
	
	tell application "Mail"
		repeat with acct in accounts
			repeat with box in mailboxes of acct
				set mailboxName to name of box
				if my isInboxMailbox(mailboxName) then
					set unreadMessages to messages of box whose read status is false
					repeat with msg in unreadMessages
						if (count of my rows) >= messageLimit then exit repeat
						set msgID to id of msg
						if my seenIDs does not contain msgID then
							set end of my seenIDs to msgID
							set msgSubject to subject of msg
							set msgSender to sender of msg
							set msgDate to date received of msg
							set row to "{\"id\":" & msgID & ",\"subject\":\"" & my jsonEscape(msgSubject) & "\",\"sender\":\"" & my jsonEscape(msgSender) & "\",\"date\":\"" & my isoDate(msgDate) & "\",\"preview\":\"\",\"mailbox\":\"" & my jsonEscape(mailboxName) & "\",\"isUnread\":true}"
							set end of my rows to row
						end if
					end repeat
				end if
				if (count of my rows) >= messageLimit then exit repeat
			end repeat
			if (count of my rows) >= messageLimit then exit repeat
		end repeat
	end tell
	
	set AppleScript's text item delimiters to ","
	set output to "[" & (my rows as text) & "]"
	set AppleScript's text item delimiters to ""
	return output
end run

on isInboxMailbox(mailboxName)
	set nameValue to mailboxName as text
	return nameValue is "INBOX" or nameValue is "Inbox" or nameValue is "Входящие"
end isInboxMailbox

on boolJSON(value)
	if value then return "true"
	return "false"
end boolJSON
