package main

import (
	"context"
	"fmt"
	"log"
	"net"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Challenge struct {
	Name        string `bson:"name"`
	Description string `bson:"description"`
	Flag        string `bson:"flag"`
	Point       int    `bson:"point"`
}

type User struct {
	Username   string `bson:"username"`
	MACAddress string `bson:"mac_address"`
}

func findUserByMAC(client *mongo.Client, macAddr string) (User, error) {
	collection := client.Database("goctf").Collection("users")
	ctx := context.Background()

	var user User
	err := collection.FindOne(ctx, bson.M{"mac_address": macAddr}).Decode(&user)
	if err != nil {
		return User{}, err
	}

	return user, nil
}

func promptUsername() string {
	var username string
	fmt.Print("Enter your username: ")
	_, _ = fmt.Scan(&username)
	return username
}

func addUser(client *mongo.Client, username, macAddr string) error {
	collection := client.Database("goctf").Collection("users")
	ctx := context.Background()

	user := User{
		Username:   username,
		MACAddress: macAddr,
	}

	_, err := collection.InsertOne(ctx, user)
	if err != nil {
		return err
	}

	return nil
}

func connectDB() (*mongo.Client, error) {
	clientOptions := options.Client().ApplyURI("mongodb://localhost:27017")
	client, err := mongo.Connect(context.Background(), clientOptions)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func showLeaderboard(client *mongo.Client) error {
	correctAnswersColl := client.Database("goctf").Collection("correctanswers")
	usersColl := client.Database("goctf").Collection("users")
	ctx := context.Background()

	pipeline := mongo.Pipeline{
		bson.D{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$mac_address"},
			{Key: "totalPoints", Value: bson.D{{Key: "$sum", Value: "$points"}}},
		}}},
		bson.D{{Key: "$sort", Value: bson.D{{Key: "totalPoints", Value: -1}}}},
	}

	cursor, err := correctAnswersColl.Aggregate(ctx, pipeline)
	if err != nil {
		return fmt.Errorf("error retrieving leaderboard data: %v", err)
	}
	defer cursor.Close(ctx)

	var leaderboard []struct {
		MACAddress  string `bson:"_id"`
		TotalPoints int    `bson:"totalPoints"`
	}

	if err := cursor.All(ctx, &leaderboard); err != nil {
		return fmt.Errorf("error decoding leaderboard data: %v", err)
	}

	fmt.Println("Leaderboard:")
	for rank, entry := range leaderboard {
		var user User
		err := usersColl.FindOne(ctx, bson.M{"mac_address": entry.MACAddress}).Decode(&user)
		if err != nil {
			fmt.Printf("%d. MAC Address: %s, Total Points: %d\n", rank+1, entry.MACAddress, entry.TotalPoints)
			continue
		}
		fmt.Printf("%d. Username: %s, Total Points: %d\n", rank+1, user.Username, entry.TotalPoints)
	}

	return nil
}

func getChallenges(client *mongo.Client) ([]Challenge, error) {
	var challenges []Challenge

	collection := client.Database("goctf").Collection("challenges")
	ctx := context.Background()
	cur, err := collection.Find(ctx, bson.D{})
	if err != nil {
		return nil, fmt.Errorf("error finding documents: %v", err)
	}
	defer cur.Close(ctx)

	for cur.Next(ctx) {
		var challenge Challenge
		err := cur.Decode(&challenge)
		if err != nil {
			return nil, fmt.Errorf("error decoding document: %v", err)
		}
		challenges = append(challenges, challenge)
	}
	if err := cur.Err(); err != nil {
		return nil, fmt.Errorf("cursor error: %v", err)
	}
	return challenges, nil
}

func displayChallenges(challenges []Challenge) {
	fmt.Println("Available Challenges:")
	for i, challenge := range challenges {
		fmt.Printf("%d. %s\n", i+1, challenge.Name)
	}
}

func recordCorrectAnswer(client *mongo.Client, macAddr, challengeName string, points int) error {
	collection := client.Database("goctf").Collection("correctanswers")
	ctx := context.Background()

	data := bson.D{
		{Key: "mac_address", Value: macAddr},
		{Key: "challenge_name", Value: challengeName},
		{Key: "points", Value: points},
	}

	_, err := collection.InsertOne(ctx, data)
	if err != nil {
		return fmt.Errorf("error recording correct answer: %v", err)
	}
	return nil
}

func main() {
	client, err := connectDB()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Disconnect(context.Background())

	for {

		interfaces, err := net.Interfaces()
		if err != nil {
			fmt.Println("Error:", err)
			continue
		}
		var macAddr string
		for _, inter := range interfaces {
			mac := inter.HardwareAddr
			if mac != nil {
				macAddr = mac.String()
				break
			}
		}

		user, err := findUserByMAC(client, macAddr)
		if err != nil {
			fmt.Println("MAC address not found in the database.")
			username := promptUsername()

			err = addUser(client, username, macAddr)
			if err != nil {
				fmt.Println("Error adding user:", err)
				continue
			}

			fmt.Println("User added successfully!")
		} else {
			fmt.Println("Welcome back,", user.Username)
		}

		fmt.Println("Choose an option:")
		fmt.Println("1. Attempt a challenge")
		fmt.Println("2. Show leaderboard")
		fmt.Println("3. Exit")

		var option int
		fmt.Print("Enter your choice: ")
		_, err = fmt.Scan(&option)
		if err != nil {
			fmt.Println("Invalid choice. Please enter a valid option.")
			continue
		}

		switch option {
		case 1:
			challenges, err := getChallenges(client)
			if err != nil {
				log.Fatal(err)
			}

			displayChallenges(challenges)

			var choice int
			fmt.Print("Enter the challenge number you want to attempt: ")
			_, err = fmt.Scan(&choice)
			if err != nil || choice < 1 || choice > len(challenges) {
				fmt.Println("Invalid choice. Please enter a valid challenge number.")
				continue
			}

			selectedChallenge := challenges[choice-1]

			fmt.Println("You selected:", selectedChallenge.Name)
			fmt.Println("Description:", selectedChallenge.Description)

			var userAnswer string
			fmt.Print("Enter the flag: ")
			_, _ = fmt.Scan(&userAnswer)

			if userAnswer == selectedChallenge.Flag {
				pointsEarned := selectedChallenge.Point

				interfaces, err := net.Interfaces()
				if err != nil {
					fmt.Println("Error:", err)
					continue
				}
				var macAddr string
				for _, inter := range interfaces {
					mac := inter.HardwareAddr
					if mac != nil {
						macAddr = mac.String()
						break
					}
				}

				err = recordCorrectAnswer(client, macAddr, selectedChallenge.Name, pointsEarned)
				if err != nil {
					fmt.Println("Error recording correct answer:", err)
					continue
				}
				fmt.Println("Correct answer recorded successfully!")
			} else {
				fmt.Println("Incorrect flag. Try again!")
				// Allow user to retry or display additional information
			}

		case 2:
			err = showLeaderboard(client)
			if err != nil {
				fmt.Println("Error displaying leaderboard:", err)
			}

		case 3:
			fmt.Println("Exiting the program.")
			return

		default:
			fmt.Println("Invalid option. Please select a valid option.")
		}
	}
}
