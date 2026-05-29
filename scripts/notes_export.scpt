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

on trimText(value)
	set textValue to value as text
	set textValue to my replaceText(textValue, return, " ")
	set textValue to my replaceText(textValue, linefeed, " ")
	set textValue to my replaceText(textValue, tab, " ")
	repeat while textValue starts with " "
		if (length of textValue) is 1 then return ""
		set textValue to text 2 thru -1 of textValue
	end repeat
	repeat while textValue ends with " "
		if (length of textValue) is 1 then return ""
		set textValue to text 1 thru -2 of textValue
	end repeat
	return textValue
end trimText

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
	set targetFolder to "Notes"
	if (count of argv) > 0 then set targetFolder to item 1 of argv
	set rows to {}
	
	tell application "Notes"
		set matchedFolders to every folder whose name is targetFolder
		if (count of matchedFolders) is 0 then error "Notes folder not found: " & targetFolder
		set selectedFolder to item 1 of matchedFolders
		repeat with noteItem in notes of selectedFolder
			set noteTitle to name of noteItem
			set noteBody to plaintext of noteItem
			set noteBody to my replaceText(noteBody, "￼", " ")
			if noteBody starts with noteTitle then
				if (length of noteBody) is greater than (length of noteTitle) then
					set noteBody to text ((length of noteTitle) + 1) thru -1 of noteBody
				else
					set noteBody to ""
				end if
			end if
			set noteBody to my replaceText(noteBody, "￼", " ")
			set noteBody to my trimText(noteBody)
			if (length of noteBody) > 240 then set noteBody to text 1 thru 240 of noteBody
			set updatedJSON to "null"
			try
				set updatedJSON to "\"" & my isoDate(modification date of noteItem) & "\""
			end try
			set row to "{\"title\":\"" & my jsonEscape(noteTitle) & "\",\"bodyPreview\":\"" & my jsonEscape(noteBody) & "\",\"updatedAt\":" & updatedJSON & ",\"folder\":\"" & my jsonEscape(targetFolder) & "\"}"
			set end of rows to row
		end repeat
	end tell
	
	set AppleScript's text item delimiters to ","
	set output to "[" & (rows as text) & "]"
	set AppleScript's text item delimiters to ""
	return output
end run
