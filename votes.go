/* 	vote.go
_________________________________
Parses votes and executes them for Kylixor Discord Bot
Andrew Langhill
kylixor.com
*/

package main

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/bwmarrin/discordgo"
)

// "0️⃣"
var numBlocks = []string{"1️⃣", "2️⃣", "3️⃣", "4️⃣", "5️⃣", "6️⃣", "7️⃣", "8️⃣", "9️⃣"}

// StartVote - begin a vote with variable vote options
func StartVote(s *discordgo.Session, m *discordgo.MessageCreate, data string, quote bool) {
	// Parse the incoming command into an array of options
	array := strings.SplitAfter(data, "|")
	options := len(array)

	if options > 9 {
		s.ChannelMessageSend(m.ChannelID, "9 options max, since Discord only has 9 number emotes")
		return
	}

	s.ChannelMessageSend(m.ChannelID, "```\nStarting vote...react now!\n```")

	// Send vote text
	voteMsg := VotePrint(s, m, data, quote)

	// Insert the vote into the vote table
	k.kdb.CreateVote(voteMsg.ID, m.GuildID, m.Author.ID, options, quote, data)

	// Upsert the vote in the watch table
	k.kdb.CreateWatch(voteMsg.ID, "vote")
}

//ReactionAdd - add a reaction to the passed-in message
func ReactionAdd(s *discordgo.Session, m *discordgo.Message, reaction string) {

	switch reaction {
	case "UPVOTE":
		err := s.MessageReactionAdd(m.ChannelID, m.ID, "⬆️")
		if err != nil {
			s.ChannelMessageSend(m.ChannelID, "Unable to use upvote emote")
		}

	case "DOWNVOTE":
		err := s.MessageReactionAdd(m.ChannelID, m.ID, "⬇️")
		if err != nil {
			s.ChannelMessageSend(m.ChannelID, "Unable to use downvote emote")
		}
	default:
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Unable to post emote: %s", reaction))
	}
}

// HandleVote - check if a vote is valid and process accordingly
func (vote *Vote) HandleVote(s *discordgo.Session, r *discordgo.MessageReactionAdd) {
	msg, err := s.ChannelMessage(r.ChannelID, r.MessageID)
	if err != nil {
		panic(err)
	}

	logInfo := fmt.Sprintf("[Reaction] \"%s\" by [%s] found on message %s", r.Emoji.Name, r.UserID, r.MessageID)
	k.Log("VOTE", logInfo)

	for i, react := range msg.Reactions {
		if react.Me && react.Count > k.botConfig.MinVotes {
			vote.Result = i + 1
			vote.UpdateVote()
		}

		if react.Me && r.UserID == vote.SubmitterID && vote.Quote && react.Emoji.Name == "⬇️" {
			vote.EndVote()
		}
	}

	if vote.Quote && vote.Result != -1 {
		if vote.Result == 1 {
			// Print finalized quote with identifier
			s.ChannelMessageSend(r.ChannelID, "Vote succeeded, yay!")
			k.kdb.DeleteWatch(r.MessageID)
			quoteAdded := k.kdb.CreateQuote(s, vote.GuildID, vote.VoteText)
			quoteAdded.RequestIdentifier(s, r)

		} else {
			s.ChannelMessageSend(r.ChannelID, "Vote failed, yikes!")
			k.kdb.DeleteWatch(r.MessageID)
		}
		return
	}

	k.Log("TEST", vote.VoteText)

	if strings.Contains(vote.VoteText, "👢") && vote.Result == 1 {
		guild, err := s.Guild(r.GuildID)
		if err != nil {
			panic(err)
		}

		// Pull userID out of the  voteText
		magic := regexp.MustCompile(`\((.*?)\)`)
		result := magic.FindStringSubmatch(vote.VoteText)
		userID := result[1]

		err = s.GuildMemberMove(r.GuildID, userID, &guild.AfkChannelID)
		if err != nil {
			s.ChannelMessageSend(r.ChannelID, "ERR: Unable to 'kick' member: "+err.Error())
		} else {
			s.ChannelMessageSend(r.ChannelID, "See ya!")
		}
		vote.EndVote()
		k.kdb.DeleteWatch(r.MessageID)
		return
	}

	if strings.Contains(vote.VoteText, "✨") {
		if vote.Result == 1 {
			guild := k.kdb.ReadGuild(s, r.GuildID)

			err := s.GuildMemberRoleAdd(r.GuildID, vote.SubmitterID, guild.MemberRole)
			if err != nil {
				s.ChannelMessageSend(r.ChannelID, "ERR: Unable to 'add' member: "+err.Error())
			} else {
				s.ChannelMessageSend(r.ChannelID, "Added member successfully!")
			}
			vote.EndVote()
			k.kdb.DeleteWatch(r.MessageID)
		}
		return
	}

	if vote.Result >= 0 {
		optionArray := strings.SplitAfter(vote.VoteText, "|")
		option := strings.TrimSpace(strings.TrimRight(optionArray[vote.Result-1], "|"))

		s.ChannelMessageSend(r.ChannelID, fmt.Sprintf("```\nOption %d, \"%s\", wins the vote!\n```", vote.Result, option))
		vote.EndVote()
		k.kdb.DeleteWatch(r.MessageID)
	}
}

// VotePrint - print out a vote and add reactions
func VotePrint(s *discordgo.Session, m *discordgo.MessageCreate, voteText string, quote bool) (message *discordgo.Message) {
	var err error
	array := strings.SplitAfter(voteText, "|")

	if quote {
		voteMsg := "```\n" + voteText + "\n```"
		message, err = s.ChannelMessageSend(m.ChannelID, voteMsg)
		if err != nil {
			panic(err)
		}
	} else {

		voteMsg := "```\n"
		for i, option := range array {
			cleanOption := strings.TrimSpace(strings.TrimRight(option, "|"))
			voteIndex := ""
			if len(array) > 1 {
				voteIndex = fmt.Sprintf("%d. ", i+1)
			}
			voteMsg += fmt.Sprintf("%s%s\n", voteIndex, cleanOption)
		}
		voteMsg += "```"

		message, err = s.ChannelMessageSend(m.ChannelID, voteMsg)
		if err != nil {
			panic(err)
		}
	}

	// Add reactions (up/down for single or numbers per option)
	if len(array) == 1 {
		err := s.MessageReactionAdd(m.ChannelID, message.ID, "⬆️")
		if err != nil {
			panic(err)
		}
		err = s.MessageReactionAdd(m.ChannelID, message.ID, "⬇️")
		if err != nil {
			panic(err)
		}
		return message
	}

	for i := range array {
		err := s.MessageReactionAdd(m.ChannelID, message.ID, numBlocks[i])
		if err != nil {
			panic(err)
		}
	}

	return message
}
